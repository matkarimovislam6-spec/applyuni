package portal

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
)

// standInPlanner plays Claude's part for tests: it reads the page snapshot
// the way the real planner is asked to (labels, ids, page text) and proposes
// actions. It knows nothing about the fake portal's code, only what the
// snapshot shows.
type standInPlanner struct {
	calls  int
	inputs []PlanInput
	// override lets a test replace the plan for a page title.
	override func(PlanInput) *PagePlan
}

var appNumber = regexp.MustCompile(`APP-\d+-\d+`)

func (p *standInPlanner) PlanPage(_ context.Context, in PlanInput) (*PagePlan, error) {
	p.calls++
	p.inputs = append(p.inputs, in)
	if p.override != nil {
		if plan := p.override(in); plan != nil {
			return plan, nil
		}
	}
	pg := in.Page
	find := func(words ...string) string {
		for _, f := range pg.Fields {
			hay := strings.ToLower(f.Selector + " " + f.Label)
			for _, w := range words {
				if strings.Contains(hay, w) {
					return f.Selector
				}
			}
		}
		return ""
	}
	var acts []Action
	add := func(a Action) {
		if a.Selector != "" || a.Kind == ActDone {
			acts = append(acts, a)
		}
	}
	text := strings.ToLower(pg.Text)
	switch {
	case strings.Contains(text, "submitted"):
		add(Action{Kind: ActDone, Confirmation: appNumber.FindString(pg.Text), Reason: "Confirmation"})
	case strings.Contains(text, "create your account"):
		add(Action{Kind: ActFill, Selector: find("email"), Field: "student.email", Reason: "Email"})
		add(Action{Kind: ActFill, Selector: find("password"), Field: "portal.password", Reason: "Password"})
		add(Action{Kind: ActClick, Selector: find("create"), Reason: "Create account"})
	case strings.Contains(text, "verify your email"):
		add(Action{Kind: ActOTP, Selector: find("code"), Reason: "Verification code"})
		add(Action{Kind: ActClick, Selector: find("verify"), Reason: "Verify"})
	case strings.Contains(text, "personal details"):
		add(Action{Kind: ActFill, Selector: find("first"), Field: "student.first_name", Reason: "First name"})
		add(Action{Kind: ActFill, Selector: find("last"), Field: "student.last_name", Reason: "Last name"})
		for _, f := range pg.Fields {
			for _, o := range f.Options {
				if o.Text == "Uzbekistan" {
					add(Action{Kind: ActChoose, Selector: f.Selector, Option: o.Value, Reason: "Citizenship"})
				}
			}
		}
		add(Action{Kind: ActClick, Selector: find("next"), Reason: "Next"})
	case strings.Contains(text, "education"):
		add(Action{Kind: ActFill, Selector: find("university"), Field: "profile.university", Reason: "University"})
		add(Action{Kind: ActFill, Selector: find("gpa", "grade"), Field: "profile.gpa", Reason: "GPA"})
		add(Action{Kind: ActTick, Selector: find("declare"), Reason: "Declaration"})
		add(Action{Kind: ActClick, Selector: find("next"), Reason: "Next"})
	case strings.Contains(text, "documents"):
		add(Action{Kind: ActUpload, Selector: find("passport"), DocType: "passport", Reason: "Passport"})
		add(Action{Kind: ActUpload, Selector: find("transcript"), DocType: "bachelor_transcript", Reason: "Transcript"})
		add(Action{Kind: ActClick, Selector: find("next", "upload"), Reason: "Upload"})
	case strings.Contains(text, "review"):
		add(Action{Kind: ActSubmit, Selector: find("submit"), Reason: "Submit application"})
	default:
		add(Action{Kind: ActNeedsHuman, Reason: "Unknown page: " + pg.Title})
	}
	return &PagePlan{Actions: acts}, nil
}

func newAgent(t *testing.T, inbox Inbox, p Planner) *Agent {
	return &Agent{Runner: newRunner(t, inbox), Planner: p}
}

func TestAgentAppliesOnUnmappedPortal(t *testing.T) {
	inbox := NewMemoryInbox()
	fp, srv := newFakePortal(inbox)
	defer srv.Close()
	planner := &standInPlanner{}

	res, err := newAgent(t, inbox, planner).Run(context.Background(), srv.URL+"/register", bundle(true), RunOptions{})
	if err != nil {
		t.Fatalf("agent run: %v", err)
	}
	if res.PortalRef != "APP-2027-0042" || fp.submitted != 1 {
		t.Fatalf("ref %q, submitted %d", res.PortalRef, fp.submitted)
	}
	if fp.form["first_name"] != "Nilufar" || fp.form["citizenship"] != "UZ" || fp.form["gpa"] != "3.4/4" || fp.form["declaration"] != "yes" {
		t.Fatalf("form %v", fp.form)
	}
	if string(fp.files["passport"]) != string(passportPDF) {
		t.Fatal("passport not uploaded")
	}
	// The planner never saw the real password.
	for _, in := range planner.inputs {
		if strings.Contains(in.Fields["portal.password"], "Pw-Strong") {
			t.Fatal("password was shown to the planner")
		}
	}

	// What the agent did is a valid Flow that the plain Runner can repeat on
	// a fresh portal without any AI.
	learned := res.Learned
	if err := learned.Validate(); err != nil {
		t.Fatalf("learned flow invalid: %v\n%+v", err, learned.Steps)
	}
	learned.Steps[0].Value = ""
	inbox2 := NewMemoryInbox()
	fp2, srv2 := newFakePortal(inbox2)
	defer srv2.Close()
	learned.Steps[0].Value = srv2.URL + "/register"
	learned.Steps = append(learned.Steps, Step{Kind: Read, Name: "Application number", Selector: "#app-number", Into: "confirmation"})
	res2, err := newRunner(t, inbox2).Run(context.Background(), learned, bundle(true), RunOptions{})
	if err != nil {
		t.Fatalf("replaying learned flow: %v", err)
	}
	if fp2.submitted != 1 || res2.PortalRef != "APP-2027-0042" {
		t.Fatalf("replay: submitted %d ref %q", fp2.submitted, res2.PortalRef)
	}
}

func TestAgentDryRunNeverSubmits(t *testing.T) {
	inbox := NewMemoryInbox()
	fp, srv := newFakePortal(inbox)
	defer srv.Close()
	res, err := newAgent(t, inbox, &standInPlanner{}).Run(context.Background(), srv.URL+"/register", bundle(false), RunOptions{DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if fp.submitted != 0 || res.Submitted {
		t.Fatal("dry run submitted")
	}
	if !strings.Contains(res.Note, "Dry run") || len(res.Receipt) == 0 {
		t.Fatalf("note %q, receipt %d bytes", res.Note, len(res.Receipt))
	}
}

func TestAgentRefusesWithoutApproval(t *testing.T) {
	a := &Agent{Runner: &Runner{}, Planner: &standInPlanner{}}
	if _, err := a.Run(context.Background(), "http://127.0.0.1:1/", bundle(false), RunOptions{}); !errors.Is(err, ErrNotApproved) {
		t.Fatalf("got %v", err)
	}
}

func TestAgentAdaptsToRedesignedPage(t *testing.T) {
	inbox := NewMemoryInbox()
	fp, srv := newFakePortal(inbox)
	defer srv.Close()
	fp.renamed = true // the GPA field has a new id; the hand-written flow breaks on this
	res, err := newAgent(t, inbox, &standInPlanner{}).Run(context.Background(), srv.URL+"/register", bundle(true), RunOptions{})
	if err != nil {
		t.Fatalf("agent run: %v", err)
	}
	if fp.form["gpa"] != "3.4/4" || res.PortalRef == "" {
		t.Fatalf("form %v ref %q", fp.form, res.PortalRef)
	}
}

func TestAgentPausesForAPerson(t *testing.T) {
	inbox := NewMemoryInbox()
	_, srv := newFakePortal(inbox)
	defer srv.Close()
	p := &standInPlanner{override: func(in PlanInput) *PagePlan {
		if strings.Contains(strings.ToLower(in.Page.Text), "education") {
			return &PagePlan{Actions: []Action{{Kind: ActNeedsHuman, Reason: "The page asks for a CAPTCHA"}}}
		}
		return nil
	}}
	_, err := newAgent(t, inbox, p).Run(context.Background(), srv.URL+"/register", bundle(true), RunOptions{})
	var se *StepError
	if !errors.Is(err, ErrNeedsHuman) || !errors.As(err, &se) || len(se.Screenshot) == 0 {
		t.Fatalf("got %v", err)
	}
}

// A planner misled by the page (or simply wrong) cannot make the agent do
// anything outside the rules.
func TestAgentRejectsUnsafePlans(t *testing.T) {
	snap := &PageSnapshot{Text: "Review. Reference APP-1", Fields: []PageField{
		{Selector: "#name", Tag: "input", Type: "text"},
		{Selector: "#country", Tag: "select", Options: []Option{{Value: "UZ", Text: "Uzbekistan"}}},
		{Selector: "#cv", Tag: "input", Type: "file"},
		{Selector: "#next", Tag: "button", Type: "submit"},
	}}
	b := bundle(true)
	files := map[string]string{"passport": "/tmp/p.pdf"}
	bad := map[string][]Action{
		"invented value":         {{Kind: ActFill, Selector: "#name", Field: "attacker.text"}},
		"selector not on page":   {{Kind: ActFill, Selector: "#evil", Field: "student.first_name"}},
		"option not offered":     {{Kind: ActChoose, Selector: "#country", Option: "RU"}},
		"unapproved document":    {{Kind: ActUpload, Selector: "#cv", DocType: "bank_statement"}},
		"fill into file input":   {{Kind: ActFill, Selector: "#cv", Field: "student.first_name"}},
		"click before the end":   {{Kind: ActClick, Selector: "#next"}, {Kind: ActFill, Selector: "#name", Field: "student.first_name"}},
		"confirmation not shown": {{Kind: ActDone, Confirmation: "APP-999"}},
		"empty plan":             {},
	}
	for name, acts := range bad {
		if err := checkPlan(&PagePlan{Actions: acts}, snap, b, files); err == nil {
			t.Errorf("%s: plan was accepted", name)
		}
	}
	ok := []Action{
		{Kind: ActFill, Selector: "#name", Field: "student.first_name"},
		{Kind: ActChoose, Selector: "#country", Option: "UZ"},
		{Kind: ActUpload, Selector: "#cv", DocType: "passport"},
		{Kind: ActClick, Selector: "#next"},
	}
	if err := checkPlan(&PagePlan{Actions: ok}, snap, b, files); err != nil {
		t.Fatalf("good plan rejected: %v", err)
	}
}
