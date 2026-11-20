package matching

import (
	"testing"
	"time"
)

var now = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

func days(n int) *int { return &n }

var cseMaster = Program{Degree: "master", Language: "en", Meta: Meta{
	Field: "computer_science", BackgroundFields: []string{"computer_science", "electrical_engineering"},
	AdjacentFields: []string{"mathematics", "physics", "statistics_data", "engineering_other"},
}}
var openSoon = Deadline{State: "open", DaysLeft: days(12), Closes: "1 Dec 2026"}

func find(r Result, key string) Check {
	for _, c := range r.Checks {
		if c.Key == key {
			return c
		}
	}
	return Check{}
}

func TestStrongCandidate(t *testing.T) {
	p := Profile{EducationLevel: "bachelor_done", Major: "computer_science", GPA: "3.7/4",
		EnglishTest: "IELTS", EnglishScore: "7.5", EnglishDate: "2026-03"}
	r := Evaluate(p, cseMaster, Deadline{State: "upcoming", Closes: "25 Feb 2027"}, now)
	if r.Verdict != "strong" || r.Score < 85 || r.Confidence != 100 {
		t.Fatalf("got %s %d conf %d: %+v", r.Verdict, r.Score, r.Confidence, r.Checks)
	}
}

// Like the screenshot: master's target, GPA 3.33/4, no English test yet.
func TestNoEnglishYet(t *testing.T) {
	p := Profile{DegreeTarget: "master", EducationLevel: "bachelor_done", Major: "computer_science", GPA: "3.33/4"}
	r := Evaluate(p, cseMaster, openSoon, now)
	if c := find(r, "language"); c.Status != Action || c.Fix == "" {
		t.Errorf("missing English should be an action with a fix: %+v", c)
	}
	if c := find(r, "grades"); c.Status != Met { // 83% of the scale = good
		t.Errorf("3.33/4 should count as good grades: %+v", c)
	}
	if c := find(r, "deadline"); c.Status != Action {
		t.Errorf("12 days left should prompt action: %+v", c)
	}
	if r.Verdict == "not_eligible" || r.Verdict == "strong" {
		t.Errorf("expected possible/long shot, got %s (%d)", r.Verdict, r.Score)
	}
}

func TestBlockers(t *testing.T) {
	// master's without a bachelor's
	r := Evaluate(Profile{EducationLevel: "high_school"}, cseMaster, openSoon, now)
	if r.Verdict != "not_eligible" {
		t.Errorf("high school → master should be not eligible, got %s", r.Verdict)
	}
	// bachelor with 11 years of school and no university year
	bsc := Program{Degree: "bachelor", Language: "en", Meta: Meta{Field: "computer_science"}}
	r = Evaluate(Profile{SchoolYears: 11}, bsc, openSoon, now)
	if c := find(r, "eligibility"); c.Status != Blocker || r.Verdict != "not_eligible" {
		t.Errorf("11 years should block: %+v", c)
	}
	r = Evaluate(Profile{SchoolYears: 11, CompletedUniYears: 1}, bsc, openSoon, now)
	if c := find(r, "eligibility"); c.Status != Met {
		t.Errorf("11 years + 1 uni year should pass: %+v", c)
	}
	// all rounds closed
	r = Evaluate(Profile{EducationLevel: "bachelor_done", Major: "computer_science"}, cseMaster, Deadline{State: "closed"}, now)
	if r.Verdict != "closed" {
		t.Errorf("closed rounds → verdict closed, got %s", r.Verdict)
	}
}

func TestFieldFit(t *testing.T) {
	for major, want := range map[string]Status{"computer_science": Met, "mathematics": Warning, "law": Warning, "": Unknown} {
		r := Evaluate(Profile{EducationLevel: "bachelor_done", Major: major}, cseMaster, openSoon, now)
		if c := find(r, "field"); c.Status != want {
			t.Errorf("major %q: %s, want %s", major, c.Status, want)
		}
	}
	// adjacent scores better than unrelated
	adj := Evaluate(Profile{EducationLevel: "bachelor_done", Major: "mathematics", GPA: "3.5/4"}, cseMaster, openSoon, now)
	far := Evaluate(Profile{EducationLevel: "bachelor_done", Major: "law", GPA: "3.5/4"}, cseMaster, openSoon, now)
	if adj.Score <= far.Score {
		t.Errorf("adjacent (%d) should beat unrelated (%d)", adj.Score, far.Score)
	}
}

func TestEnglish(t *testing.T) {
	cases := []struct {
		test, score, date string
		want              Status
	}{
		{"IELTS", "6.5", "2026-01", Met},
		{"IELTS", "5.5", "2026-01", Warning},
		{"IELTS", "5.0", "2026-01", Action},
		{"TOEFL", "95", "2026-01", Met},
		{"Duolingo", "100", "2026-01", Warning},
		{"Cambridge", "C1", "2025-06", Met},
		{"IELTS", "7.0", "2024-05", Action}, // older than 2 years
		{"IELTS", "abc", "", Unknown},
	}
	for _, c := range cases {
		r := Evaluate(Profile{EducationLevel: "bachelor_done", EnglishTest: c.test, EnglishScore: c.score, EnglishDate: c.date}, cseMaster, openSoon, now)
		if got := find(r, "language"); got.Status != c.want {
			t.Errorf("%s %s (%s): %s, want %s — %s", c.test, c.score, c.date, got.Status, c.want, got.Detail)
		}
	}
	// a verified programme minimum overrides the general norm
	strict := cseMaster
	strict.Meta.EnglishMinIELTS = 7
	r := Evaluate(Profile{EducationLevel: "bachelor_done", EnglishTest: "IELTS", EnglishScore: "6.5"}, strict, openSoon, now)
	if c := find(r, "language"); c.Status != Warning {
		t.Errorf("6.5 vs verified minimum 7.0 should warn: %+v", c)
	}
}

func TestGradePct(t *testing.T) {
	cases := map[string]float64{"3.33/4": 0.8325, "87/100": 0.87, "4.6/5": 0.92, "4,5 / 5": 0.9, "8.1/10": 0.81, "90%": 0.9, "4.6": 0.92}
	for in, want := range cases {
		got, ok := GradePct(in)
		if !ok || got < want-0.001 || got > want+0.001 {
			t.Errorf("GradePct(%q) = %v %v, want %v", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "abc", "5/4", "0"} {
		if _, ok := GradePct(bad); ok {
			t.Errorf("GradePct(%q) should fail", bad)
		}
	}
}

func TestEntryTestAndPortfolio(t *testing.T) {
	prog := Program{Degree: "bachelor", Language: "en", Meta: Meta{Field: "architecture_design", NeedsPortfolio: true, EntryTest: "CISIA test"}}
	r := Evaluate(Profile{SchoolYears: 12, Interests: []string{"architecture_design"}}, prog, openSoon, now)
	if find(r, "entry_test").Status != Action || find(r, "portfolio").Status != Action {
		t.Fatalf("expected entry test and portfolio actions: %+v", r.Checks)
	}
	with := Evaluate(Profile{SchoolYears: 12, Interests: []string{"architecture_design"}, HasPortfolio: true}, prog, openSoon, now)
	if with.Score <= r.Score {
		t.Errorf("having a portfolio should raise the score (%d vs %d)", with.Score, r.Score)
	}
}
