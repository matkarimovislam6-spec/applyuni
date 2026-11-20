// Package matching pre-screens a student against a programme: is the student
// eligible, how well do they fit, and what must they fix? It is rule-based on
// purpose (cheap, fast, explainable); every point of the score has a reason.
//
// Programme-specific minimums (GPA, IELTS) are used only when the programme's
// requirements state them; otherwise general Italian norms are applied and the
// check says the programme's own minimum is not verified.
package matching

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Profile is what the student told us (stored as JSON on the student).
type Profile struct {
	DegreeTarget      string   `json:"degree_target"`   // bachelor | master
	EducationLevel    string   `json:"education_level"` // high_school | bachelor_in_progress | bachelor_done | master_done
	SchoolYears       int      `json:"school_years"`
	CompletedUniYears int      `json:"completed_university_years"`
	SchoolGrade       string   `json:"school_grade"` // e.g. "4.6/5" or "87/100"
	University        string   `json:"university"`
	Major             string   `json:"major"`      // field key, see Fields
	MajorName         string   `json:"major_name"` // free text, e.g. "Software Engineering"
	GPA               string   `json:"gpa"`        // e.g. "3.33/4"
	GraduationYear    int      `json:"graduation_year"`
	EnglishTest       string   `json:"english_test"` // IELTS | TOEFL | Duolingo | Cambridge
	EnglishScore      string   `json:"english_score"`
	EnglishDate       string   `json:"english_date"`  // YYYY-MM
	ItalianLevel      string   `json:"italian_level"` // A1..C2
	GermanLevel       string   `json:"german_level"`  // A1..C2
	OtherLanguages    []Lang   `json:"other_languages"`
	SAT               int      `json:"sat_score"`
	GRE               int      `json:"gre_score"`
	GMAT              int      `json:"gmat_score"`
	WorkYears         int      `json:"work_years"`
	HasPortfolio      bool     `json:"has_portfolio"`
	Interests         []string `json:"interests"` // field keys
	BudgetEURYear     int      `json:"budget_eur_year"`
	WantsScholarship  bool     `json:"wants_scholarship"`
	DocumentsLanguage string   `json:"documents_language"`
}

type Lang struct {
	Language string `json:"language"`
	Cert     string `json:"cert"`
	Level    string `json:"level"`
}

// Programme facts the matcher needs (from programs.requirements JSON + catalogue).
type Program struct {
	Country  string // IT | DE | US
	Degree   string
	Language string // en | it
	Meta     Meta
}

type Meta struct {
	Field            string   `json:"field"`
	BackgroundFields []string `json:"background_fields"` // master: degrees that usually qualify
	AdjacentFields   []string `json:"adjacent_fields"`   // master: may qualify after a credit check
	EnglishMinIELTS  float64  `json:"english_min_ielts"` // only when verified
	MinGPAPct        float64  `json:"min_gpa_pct"`       // only when verified
	MinGPANote       string   `json:"min_gpa_note"`
	Selectivity      int      `json:"selectivity"` // 1 very selective … 3 moderate (editorial)
	EntryTest        string   `json:"entry_test"`
	EntryTestSource  string   `json:"entry_test_source"`
	NeedsPortfolio   bool     `json:"needs_portfolio"`
}

func ParseMeta(requirements json.RawMessage) Meta {
	var m Meta
	_ = json.Unmarshal(requirements, &m)
	if !m.NeedsPortfolio && strings.Contains(string(requirements), `"portfolio"`) {
		m.NeedsPortfolio = true
	}
	return m
}

// Deadline is the slice of deadline status the matcher uses.
type Deadline struct {
	State    string // open | upcoming | closed | not_published
	DaysLeft *int
	Closes   string // human date of the next deadline
}

type Status string

const (
	Met     Status = "met"     // fine
	Action  Status = "action"  // you need to do something
	Warning Status = "warning" // risk to your chances
	Blocker Status = "blocker" // not eligible until fixed
	Unknown Status = "unknown" // tell us more
)

type Check struct {
	Key    string      `json:"key"`
	Label  string      `json:"label"`
	Status Status      `json:"status"`
	Detail string      `json:"detail"`
	Fix    string      `json:"fix,omitempty"`
	Score  *GradeScore `json:"score,omitempty"` // grades check only: the weighted breakdown
}

type Result struct {
	Score      int     `json:"score"`      // 0–100
	Verdict    string  `json:"verdict"`    // strong | possible | long_shot | not_eligible | closed
	Confidence int     `json:"confidence"` // % of checks we could actually assess
	Checks     []Check `json:"checks"`
}

// component weights (sum 100)
const (
	wFit      = 30
	wGrades   = 25
	wLanguage = 25
	wReady    = 20
)

func Evaluate(p Profile, prog Program, dl Deadline, now time.Time) Result {
	var r Result
	pts, max := 0.0, 0.0
	known, total := 0, 0
	add := func(c Check, got, weight float64) {
		r.Checks = append(r.Checks, c)
		total++
		if c.Status != Unknown {
			known++
		}
		pts += got
		max += weight
	}

	// ---- 1. Eligibility (no points, but can block)
	if c, ok := eligibility(p, prog); ok {
		r.Checks = append(r.Checks, c)
		total++
		if c.Status != Unknown {
			known++
		}
	}

	// ---- 2. Field fit
	c, f := fieldFit(p, prog)
	add(c, f*wFit, wFit)

	// ---- 3. Grades
	c, f = grades(p, prog)
	add(c, f*wGrades, wGrades)

	// ---- 4. Language
	c, f = language(p, prog, now)
	add(c, f*wLanguage, wLanguage)

	// ---- 5. Readiness: entrance test, portfolio, deadline
	ready := []Check{}
	rf := 1.0
	if prog.Meta.EntryTest != "" {
		ready = append(ready, Check{Key: "entry_test", Label: "Entrance test", Status: Action,
			Detail: prog.Meta.EntryTest, Fix: "Register for the test early: seats and sessions fill up."})
		rf -= 0.25
	}
	if prog.Meta.NeedsPortfolio {
		if p.HasPortfolio {
			ready = append(ready, Check{Key: "portfolio", Label: "Portfolio", Status: Met, Detail: "You have a portfolio ready."})
		} else {
			ready = append(ready, Check{Key: "portfolio", Label: "Portfolio", Status: Action,
				Detail: "This programme can ask for a portfolio of your work.", Fix: "Collect 10–15 of your best projects in one PDF."})
			rf -= 0.25
		}
	}
	dc, df := deadline(dl)
	ready = append(ready, dc)
	// readiness counts once: its checks are listed, the combined score is added with the last one
	for _, c := range ready[:len(ready)-1] {
		add(c, 0, 0)
	}
	add(dc, math.Max(0, rf*df)*wReady, wReady)

	if max > 0 {
		r.Score = int(math.Round(pts / max * 100))
	}
	if total > 0 {
		r.Confidence = int(math.Round(float64(known) / float64(total) * 100))
	}
	r.Verdict = verdict(r)
	return r
}

func verdict(r Result) string {
	for _, c := range r.Checks {
		if c.Status == Blocker && c.Key == "deadline" {
			return "closed"
		}
	}
	for _, c := range r.Checks {
		if c.Status == Blocker {
			return "not_eligible"
		}
	}
	// A required certificate you don't have yet (English, German, Italian)
	// rules out "strong", however good the rest looks.
	languagePending := false
	for _, c := range r.Checks {
		if c.Key == "language" && c.Status == Action {
			languagePending = true
		}
	}
	switch {
	case r.Score >= 75 && !languagePending:
		return "strong"
	case r.Score >= 75:
		return "possible"
	case r.Score >= 55:
		return "possible"
	default:
		return "long_shot"
	}
}

// ---------------- checks ----------------

func eligibility(p Profile, prog Program) (Check, bool) {
	c := Check{Key: "eligibility", Label: "Entry level"}
	if prog.Degree == "master" {
		switch p.EducationLevel {
		case "bachelor_done", "master_done":
			c.Status, c.Detail = Met, "You have a bachelor's degree."
		case "bachelor_in_progress":
			c.Status, c.Detail = Met, "You can apply in your final year; you must graduate before enrolment."
			if p.GraduationYear > 0 && p.GraduationYear > time.Now().Year()+1 {
				c.Status = Blocker
				c.Detail = fmt.Sprintf("You graduate in %d, after this intake starts.", p.GraduationYear)
				c.Fix = "Apply for the intake after your graduation."
			}
		case "high_school":
			c.Status, c.Detail = Blocker, "A master's needs a bachelor's degree."
			c.Fix = "Look at bachelor's programmes instead."
		default:
			c.Status, c.Detail, c.Fix = Unknown, "We don't know your level of education yet.", "Fill in 'Highest education' in your profile."
		}
		return c, true
	}
	if p.EducationLevel == "bachelor_done" || p.EducationLevel == "master_done" {
		c.Status, c.Detail = Met, "You already have a degree; a second bachelor's is possible but check whether a master's fits you better."
		return c, true
	}
	switch prog.Country {
	case "DE":
		// Most Uzbek school certificates don't give direct entry (DAAD admission database).
		if p.CompletedUniYears >= 1 {
			c.Status = Warning
			c.Detail = fmt.Sprintf("With %d completed university year(s) you may get direct entry, depending on your subject and years.", p.CompletedUniYears)
			c.Fix = "Check your exact case in the DAAD admission database before applying."
			return c, true
		}
		c.Status = Blocker
		c.Detail = "Most Uzbek school certificates don't give direct entry to a German bachelor's."
		c.Fix = "Do a Studienkolleg (foundation year) and its Feststellungsprüfung, or complete university study in a related subject first."
		return c, true
	case "US":
		if p.SchoolYears == 0 && p.EducationLevel == "" {
			c.Status, c.Detail, c.Fix = Unknown, "We don't know whether you've finished school yet.", "Fill in your education in your profile."
			return c, true
		}
		c.Status, c.Detail = Met, "US universities admit students who have completed (or are finishing) secondary school."
		return c, true
	}
	// Italy: 12 years of schooling
	switch {
	case p.SchoolYears == 0:
		c.Status, c.Detail, c.Fix = Unknown, "We don't know how many years of school you finished.", "Fill in 'Years of school completed'."
	case p.SchoolYears >= 12:
		c.Status, c.Detail = Met, "You have 12 years of schooling, as Italy requires."
	case p.CompletedUniYears >= 1:
		c.Status, c.Detail = Met, "11 years of school plus a completed university year meets Italy's 12-year rule."
	default:
		c.Status = Blocker
		c.Detail = "Italy requires 12 years of schooling. With 11, you need one completed year of university or a recognised foundation year."
		c.Fix = "Finish your first university year (or a foundation year) before enrolling."
	}
	return c, true
}

func fieldFit(p Profile, prog Program) (Check, float64) {
	c := Check{Key: "field", Label: "Subject fit"}
	m := prog.Meta
	if prog.Degree == "master" {
		major := p.Major
		name := FieldName(major)
		if p.MajorName != "" {
			name = p.MajorName
		}
		switch {
		case major == "":
			c.Status, c.Detail, c.Fix = Unknown, "We don't know your bachelor's subject.", "Add your major in your profile."
			return c, 0.5
		case slices.Contains(m.BackgroundFields, major):
			c.Status, c.Detail = Met, fmt.Sprintf("Your background (%s) is a usual entry route for this programme.", name)
			return c, 1
		case slices.Contains(m.AdjacentFields, major):
			c.Status = Warning
			c.Detail = fmt.Sprintf("%s is related. The university checks whether your courses cover enough of %s.", name, FieldName(m.Field))
			c.Fix = "Prepare your transcript with course descriptions (syllabi); they decide on missing credits."
			return c, 0.6
		default:
			c.Status = Warning
			c.Detail = fmt.Sprintf("Your background (%s) is far from %s; you'd likely be missing required credits.", name, FieldName(m.Field))
			c.Fix = "Consider programmes in your own field, or take the missing courses first."
			return c, 0.15
		}
	}
	// bachelor: interests
	if len(p.Interests) == 0 {
		c.Status, c.Detail, c.Fix = Unknown, "You haven't told us which subjects interest you.", "Pick your interests in your profile."
		return c, 0.6
	}
	if slices.Contains(p.Interests, m.Field) {
		c.Status, c.Detail = Met, fmt.Sprintf("Matches your interest in %s.", FieldName(m.Field))
		return c, 1
	}
	c.Status, c.Detail = Warning, fmt.Sprintf("%s isn't one of the interests you picked.", FieldName(m.Field))
	return c, 0.35
}

var fracRe = regexp.MustCompile(`^\s*([0-9]+(?:[.,][0-9]+)?)\s*(?:/|out of|of)\s*([0-9]+(?:[.,][0-9]+)?)\s*$`)

// GradePct turns "3.33/4", "87/100" or "4.6" (assumed /5 if ≤5, /100 if >10) into a 0–1 fraction.
func GradePct(s string) (float64, bool) {
	s = strings.ReplaceAll(strings.TrimSpace(s), "%", "")
	if s == "" {
		return 0, false
	}
	num := func(x string) (float64, bool) {
		v, err := strconv.ParseFloat(strings.ReplaceAll(x, ",", "."), 64)
		return v, err == nil
	}
	if m := fracRe.FindStringSubmatch(s); m != nil {
		a, ok1 := num(m[1])
		b, ok2 := num(m[2])
		if ok1 && ok2 && b > 0 && a <= b {
			return a / b, true
		}
		return 0, false
	}
	v, ok := num(s)
	if !ok || v <= 0 {
		return 0, false
	}
	switch {
	case v <= 4 && strings.Contains(s, "."):
		return v / 4, true // most likely a 4-point GPA
	case v <= 5:
		return v / 5, true // Uzbek / Russian 5-point scale
	case v <= 10:
		return v / 10, true
	case v <= 100:
		return v / 100, true
	}
	return 0, false
}

func grades(p Profile, prog Program) (Check, float64) {
	c := Check{Key: "grades", Label: "Grades"}
	raw, what := p.SchoolGrade, "school grade average"
	if prog.Degree == "master" {
		raw, what = p.GPA, "bachelor's GPA"
	}
	pct, ok := GradePct(raw)
	if !ok {
		c.Status, c.Detail = Unknown, fmt.Sprintf("We don't know your %s.", what)
		c.Fix = "Add it as a fraction, e.g. 3.4/4 or 4.5/5."
		return c, 0.5
	}
	pc := int(math.Round(pct * 100))
	if prog.Meta.MinGPAPct > 0 && pct*100 < prog.Meta.MinGPAPct {
		c.Status = Blocker
		c.Detail = fmt.Sprintf("Your %s (%d%%) is below this programme's minimum (%.0f%%).", what, pc, prog.Meta.MinGPAPct)
		if prog.Meta.MinGPANote != "" {
			c.Detail += " " + prog.Meta.MinGPANote
		}
		c.Fix = "Look at programmes without a minimum, or improve your grades if you're still studying."
		return c, 0
	}
	// weighted score: grade × origin university × programme selectivity (scale_algorithm.go)
	g := ScoreGrade(pct, p.University, prog.Degree != "master", prog.Meta.Selectivity)
	c.Detail = g.Explain(what)
	c.Score = &g
	switch g.Band {
	case "strong", "good":
		c.Status = Met
	case "borderline":
		c.Status = Warning
		c.Fix = "Strengthen the rest of your application: test scores, motivation letter, experience."
	default:
		c.Status = Warning
		c.Fix = "This programme is likely out of reach on grades; add less selective programmes to your list."
	}
	return c, g.Fraction
}

// IELTS-equivalent of the student's English test.
func ieltsEquivalent(test, score string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(score), ",", "."), 64)
	if err != nil && strings.ToLower(test) != "cambridge" {
		return 0, false
	}
	switch strings.ToLower(test) {
	case "ielts":
		return v, v > 0 && v <= 9
	case "toefl": // TOEFL iBT → IELTS (ETS comparison, rounded)
		for _, t := range []struct{ toefl, ielts float64 }{{115, 8.5}, {110, 8}, {102, 7.5}, {94, 7}, {79, 6.5}, {60, 6}, {46, 5.5}, {35, 5}} {
			if v >= t.toefl {
				return t.ielts, true
			}
		}
		return 4.5, v > 0
	case "duolingo":
		for _, t := range []struct{ d, ielts float64 }{{145, 8}, {135, 7.5}, {125, 7}, {115, 6.5}, {105, 6}, {95, 5.5}} {
			if v >= t.d {
				return t.ielts, true
			}
		}
		return 5, v > 0
	case "cambridge":
		switch strings.ToUpper(strings.TrimSpace(score)) {
		case "C2":
			return 8.5, true
		case "C1":
			return 7, true
		case "B2":
			return 6, true
		}
	}
	return 0, false
}

func language(p Profile, prog Program, now time.Time) (Check, float64) {
	if prog.Language == "de" {
		c := Check{Key: "language", Label: "German"}
		lvl := strings.ToUpper(p.GermanLevel)
		switch {
		case lvl == "":
			c.Status, c.Detail, c.Fix = Action, "This programme is taught in German.", "German-taught programmes usually ask for C1 (TestDaF, DSH-2 or Goethe C1)."
			return c, 0.2
		case lvl >= "C1":
			c.Status, c.Detail = Met, "Your German ("+lvl+") meets the usual C1 requirement."
			return c, 1
		default:
			c.Status, c.Detail, c.Fix = Warning, "German "+lvl+" is below the C1 most German-taught programmes require.", "Aim for TestDaF, DSH-2 or Goethe C1 before applying."
			return c, 0.35
		}
	}
	if prog.Language == "it" {
		c := Check{Key: "language", Label: "Italian"}
		lvl := strings.ToUpper(p.ItalianLevel)
		switch {
		case lvl == "":
			c.Status, c.Detail, c.Fix = Action, "This programme is taught in Italian.", "Most Italian-taught programmes ask for B2: plan a CILS/CELI/PLIDA certificate."
			return c, 0.2
		case lvl >= "B2":
			c.Status, c.Detail = Met, "Your Italian ("+lvl+") is enough for most Italian-taught programmes."
			return c, 1
		default:
			c.Status, c.Detail, c.Fix = Warning, "Italian "+lvl+" is usually below the B2 most programmes require.", "Aim for a B2 certificate before enrolment."
			return c, 0.4
		}
	}
	c := Check{Key: "language", Label: "English"}
	if p.EnglishTest == "" {
		c.Status = Action
		c.Detail = "English-taught programmes ask for a certificate (usually about IELTS 6.0–6.5)."
		c.Fix = "Book IELTS or TOEFL now: results take up to 2 weeks."
		return c, 0.2
	}
	ielts, ok := ieltsEquivalent(p.EnglishTest, p.EnglishScore)
	if !ok {
		c.Status, c.Detail, c.Fix = Unknown, "We couldn't read your English score.", "Enter the overall score, e.g. 6.5 for IELTS or 85 for TOEFL."
		return c, 0.5
	}
	// validity: most certificates are accepted for 2 years
	if t, err := time.Parse("2006-01", p.EnglishDate); err == nil && now.Sub(t) > 2*365*24*time.Hour {
		c.Status = Action
		c.Detail = fmt.Sprintf("Your %s from %s is older than 2 years; most universities won't accept it.", p.EnglishTest, t.Format("Jan 2006"))
		c.Fix = "Retake the test before applying."
		return c, 0.3
	}
	min, where := 6.0, "Italy"
	switch prog.Country {
	case "DE":
		min, where = 6.5, "Germany"
	case "US":
		min, where = 6.5, "the US"
	}
	minNote := fmt.Sprintf("Most universities in %s ask for about IELTS %.1f (or the TOEFL/Duolingo equivalent); this programme's exact minimum isn't verified yet.", where, min)
	if prog.Meta.EnglishMinIELTS > 0 {
		min = prog.Meta.EnglishMinIELTS
		minNote = fmt.Sprintf("This programme asks for IELTS %.1f or equivalent.", min)
	}
	eq := fmt.Sprintf("%s %s", p.EnglishTest, p.EnglishScore)
	if !strings.EqualFold(p.EnglishTest, "IELTS") {
		eq += fmt.Sprintf(" (≈ IELTS %.1f)", ielts)
	}
	switch {
	case ielts >= min+1:
		c.Status, c.Detail = Met, eq+": comfortably above the usual minimum."
		return c, 1
	case ielts >= min:
		c.Status, c.Detail = Met, eq+". "+minNote
		return c, 0.85
	case ielts >= min-0.5:
		c.Status, c.Detail, c.Fix = Warning, eq+", just below the usual minimum. "+minNote, "A retake aiming for IELTS "+strconv.FormatFloat(min, 'f', 1, 64)+"+ would make this safe."
		return c, 0.45
	default:
		c.Status, c.Detail, c.Fix = Action, eq+", below the usual minimum. "+minNote, "Retake the test aiming for IELTS "+strconv.FormatFloat(min, 'f', 1, 64)+" or more."
		return c, 0.15
	}
}

func deadline(dl Deadline) (Check, float64) {
	c := Check{Key: "deadline", Label: "Deadline"}
	switch dl.State {
	case "open", "due":
		if dl.DaysLeft != nil && *dl.DaysLeft <= 14 {
			c.Status, c.Detail, c.Fix = Action, fmt.Sprintf("Open now, closes %s (%d days left).", dl.Closes, *dl.DaysLeft), "Apply this week."
			return c, 0.9
		}
		c.Status, c.Detail = Met, "Applications are open until "+dl.Closes+"."
		return c, 1
	case "upcoming":
		c.Status, c.Detail = Met, "Next round closes "+dl.Closes+"."
		return c, 1
	case "closed":
		c.Status, c.Detail, c.Fix = Blocker, "All rounds for this intake have closed.", "Plan for the next intake."
		return c, 0
	default:
		c.Status, c.Detail = Unknown, "The university hasn't published this year's deadline yet."
		return c, 0.8
	}
}

// ---------------- fields ----------------

var fields = map[string]string{
	"computer_science":       "Computer science / IT",
	"electrical_engineering": "Electrical & electronic engineering",
	"engineering_other":      "Other engineering",
	"mathematics":            "Mathematics",
	"statistics_data":        "Statistics / data",
	"physics":                "Physics",
	"natural_sciences":       "Biology / chemistry",
	"economics_business":     "Economics / business",
	"law":                    "Law",
	"political_science":      "Politics / international relations",
	"psychology":             "Psychology",
	"social_sciences":        "Other social sciences",
	"architecture_design":    "Architecture / design",
	"humanities":             "Languages / humanities",
	"health":                 "Medicine / health",
}

func FieldName(key string) string {
	if n, ok := fields[key]; ok {
		return strings.ToLower(n[:1]) + n[1:]
	}
	if key == "" {
		return "your field"
	}
	return key
}
