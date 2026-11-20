package service

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/matkarimovislam6-spec/applyuni/backend/internal/i18n"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/matching"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/portal"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/requirements"
)

// Every text the backend can show a student must exist in Uzbek and Russian.
// This test produces that text from many profiles across the whole catalogue
// and fails on anything the catalogs don't cover.
func TestEverythingIsTranslated(t *testing.T) {
	if err := i18n.LoadError(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	s, _ := newTestService(t)
	progs, err := s.ListPrograms(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}

	texts := map[string]bool{}
	add := func(v ...string) {
		for _, x := range v {
			if strings.TrimSpace(x) != "" {
				texts[x] = true
			}
		}
	}
	addp := func(p *string) {
		if p != nil {
			add(*p)
		}
	}

	profiles := []matching.Profile{
		{},
		{DegreeTarget: "bachelor", SchoolYears: 11, SchoolGrade: "4.2/5", Interests: []string{"law"}},
		{DegreeTarget: "bachelor", SchoolYears: 11, CompletedUniYears: 1, SchoolGrade: "3.2/5", EnglishTest: "IELTS", EnglishScore: "5.0", EnglishDate: "2026-01"},
		{DegreeTarget: "bachelor", SchoolYears: 12, SchoolGrade: "95/100", Interests: []string{"computer_science", "architecture_design"}, EnglishTest: "Duolingo", EnglishScore: "100", HasPortfolio: true},
		{DegreeTarget: "master", EducationLevel: "high_school"},
		{DegreeTarget: "master", EducationLevel: "bachelor_in_progress", GraduationYear: 2031, Major: "law", GPA: "2.1/4", University: "Some College"},
		{DegreeTarget: "master", EducationLevel: "bachelor_done", Major: "mathematics", GPA: "3.0/4", University: "TUIT", EnglishTest: "IELTS", EnglishScore: "5.5", EnglishDate: "2026-02"},
		{DegreeTarget: "master", EducationLevel: "bachelor_done", Major: "computer_science", GPA: "3.9/4", University: "Technical University of Munich", EnglishTest: "TOEFL", EnglishScore: "110", EnglishDate: "2026-03", GermanLevel: "B2", ItalianLevel: "B1"},
		{DegreeTarget: "master", EducationLevel: "master_done", MajorName: "Software Engineering", Major: "computer_science", GPA: "abc", EnglishTest: "IELTS", EnglishScore: "x"},
		{DegreeTarget: "master", EducationLevel: "bachelor_done", Major: "economics_business", GPA: "60/100", EnglishTest: "Cambridge", EnglishScore: "C1", EnglishDate: "2023-01"},
	}
	at := time.Now()
	for _, p := range progs {
		addp(p.FeeNote)
		addp(p.CallNote)
		for _, d := range p.Deadlines {
			add(d.Round)
			addp(d.Note)
		}
		for _, prof := range profiles {
			raw, _ := json.Marshal(prof)
			var rp requirements.Profile
			json.Unmarshal(raw, &rp)
			for _, lang := range []string{"en", "uz", "ru", "mixed"} {
				rp.DocumentsLanguage = lang
				rp.WantsScholarship = true
				cl, err := requirements.Build(json.RawMessage(p.CountryRules), json.RawMessage(p.Requirements), rp,
					requirements.Program{Degree: p.Degree, Language: p.Language}, nil)
				if err != nil {
					t.Fatal(err)
				}
				for _, it := range cl.Items {
					add(it.Title, it.Description)
				}
				for _, st := range cl.Steps {
					add(st.Title)
				}
				add(cl.Warnings...)
			}
			m := evaluate(prof, p, at)
			for _, c := range m.Checks {
				add(c.Label, c.Detail, c.Fix)
			}
			// the other deadline states and languages too
			for _, dl := range []matching.Deadline{{State: "open", DaysLeft: ptr(3), Closes: "1 Dec 2026"}, {State: "open", DaysLeft: ptr(40), Closes: "1 Mar 2027"},
				{State: "due", DaysLeft: ptr(20), Closes: "1 Feb 2027"}, {State: "upcoming", Closes: "31 May 2027"}, {State: "closed"}, {State: "not_published"}} {
				for _, lang := range []string{"en", "it", "de"} {
					r := matching.Evaluate(prof, matching.Program{Country: p.Country, Degree: p.Degree, Language: lang, Meta: matching.ParseMeta(json.RawMessage(p.Requirements))}, dl, at)
					for _, c := range r.Checks {
						add(c.Label, c.Detail, c.Fix)
					}
				}
			}
		}
	}
	// a verified IELTS minimum, and submission outcomes
	strict := matching.Program{Country: "IT", Degree: "master", Language: "en", Meta: matching.Meta{EnglishMinIELTS: 7, MinGPAPct: 80, MinGPANote: "ASU asks for a 3.00 GPA (B or better)."}}
	for _, sc := range []string{"6.0", "6.5", "6.9", "8.5"} {
		r := matching.Evaluate(matching.Profile{EducationLevel: "bachelor_done", GPA: "2.5/4", EnglishTest: "IELTS", EnglishScore: sc}, strict, matching.Deadline{}, at)
		for _, c := range r.Checks {
			add(c.Detail, c.Fix)
		}
	}
	add(explain(&portal.MissingError{Fields: []string{"profile.gpa"}}), explain(portal.ErrNeedsHuman), explain(portal.ErrNotApproved),
		explain(&portal.StepError{Step: "GPA", Err: errors.New("x")}),
		"Automated submission to University of Pisa is not available yet. Your approved package is ready.")
	for _, e := range []error{ErrNotFound, ErrForbidden, ErrBadLogin, ErrSeatLimit, ErrNotReady} {
		add(e.Error())
	}
	add("password must be at least 8 characters", "this username is taken", "passwords don't match", "file larger than 20 MB")

	for _, lang := range []string{i18n.UZ, i18n.RU} {
		var missing []string
		for txt := range texts {
			if _, ok := i18n.Lookup(lang, txt); !ok {
				missing = append(missing, txt)
			}
		}
		sort.Strings(missing)
		if len(missing) > 0 {
			t.Errorf("%s: %d of %d texts have no translation:\n  %s", lang, len(missing), len(texts), strings.Join(missing, "\n  "))
		}
	}
	t.Logf("checked %d distinct texts", len(texts))
}

