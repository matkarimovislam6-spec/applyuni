package portal

// A Flow is a portal "recipe": the exact steps a person would take on one
// university portal, written as data. The Runner (runner.go) executes it in a
// real browser. Adding a university = writing its Flow after mapping the live
// portal with a test account; the engine itself doesn't change.

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type StepKind string

const (
	Goto   StepKind = "goto"   // open URL (may contain {{fields}})
	Fill   StepKind = "fill"   // type Value into an input
	Choose StepKind = "choose" // pick Value in a <select>
	Tick   StepKind = "tick"   // tick a checkbox
	Upload StepKind = "upload" // attach the student's document of DocType
	Click  StepKind = "click"  // click a button or link
	Expect StepKind = "expect" // check we're on the expected page (Selector visible, optional Contains text)
	OTP    StepKind = "otp"    // wait for the emailed code (CodeRe on the student's inbox) and type it into Selector
	Read   StepKind = "read"   // read text at Selector into Into (e.g. the confirmation number)
	Submit StepKind = "submit" // the one irreversible click: only with approval, never in dry run
	Human  StepKind = "human"  // a CAPTCHA or similar: pause and ask the student
)

type Step struct {
	Kind     StepKind
	Name     string // shown in logs and errors, e.g. "Personal details: first name"
	Selector string // CSS selector
	Value    string // literal or template, e.g. "{{student.first_name}}"
	DocType  string // for Upload
	CodeRe   string // for OTP: regexp with one capture group
	Into     string // for Read
	Contains string // for Expect
	Optional bool   // skip silently if Selector isn't on the page
}

type Flow struct {
	Portal string // portal id, matches universities.portal
	Name   string
	// Account registers the student on the portal and confirms their email.
	// It runs once per student and portal, right after approval; Steps then
	// start by logging in with that account. Empty if the portal has no
	// separate registration.
	Account []Step
	Steps   []Step
}

var fieldRe = regexp.MustCompile(`\{\{\s*([a-z0-9_.]+)\s*\}\}`)

// Fields lists every {{field}} the flow needs, sorted.
func (f Flow) Fields() []string {
	set := map[string]bool{}
	for _, s := range append(append([]Step{}, f.Account...), f.Steps...) {
		for _, m := range fieldRe.FindAllStringSubmatch(s.Value, -1) {
			set[m[1]] = true
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Documents lists every document type the flow uploads (non-optional ones).
func (f Flow) Documents() []string {
	var out []string
	for _, s := range f.Steps {
		if s.Kind == Upload && !s.Optional {
			out = append(out, s.DocType)
		}
	}
	return out
}

// Validate checks the flow itself is well-formed: exactly one Submit, which
// comes after everything it depends on, selectors where needed.
func (f Flow) Validate() error {
	for i, s := range f.Account {
		if s.Kind == Submit || s.Kind == Upload {
			return fmt.Errorf("account step %d (%s): registration can't upload or submit", i, s.Name)
		}
		if err := validStep(s); err != nil {
			return err
		}
	}
	submits := 0
	for i, s := range f.Steps {
		if s.Name == "" {
			return fmt.Errorf("step %d has no name", i)
		}
		if err := validStep(s); err != nil {
			return err
		}
		if s.Kind == Submit {
			submits++
		}
	}
	if submits != 1 {
		return fmt.Errorf("flow %s must have exactly one submit step, has %d", f.Portal, submits)
	}
	return nil
}

func validStep(s Step) error {
	if s.Name == "" {
		return errors.New("a step has no name")
	}
	switch s.Kind {
	case Goto:
		if s.Value == "" {
			return fmt.Errorf("step %q: goto needs a URL", s.Name)
		}
	case Fill, Choose, Tick, Upload, Click, Expect, OTP, Read, Submit:
		if s.Selector == "" {
			return fmt.Errorf("step %q: needs a selector", s.Name)
		}
	case Human:
	default:
		return fmt.Errorf("step %q: unknown kind %q", s.Name, s.Kind)
	}
	if s.Kind == Upload && s.DocType == "" {
		return fmt.Errorf("step %q: upload needs a document type", s.Name)
	}
	if s.Kind == OTP {
		re, err := regexp.Compile(s.CodeRe)
		if err != nil || re.NumSubexp() != 1 {
			return fmt.Errorf("step %q: otp needs a regexp with one capture group", s.Name)
		}
	}
	if s.Kind == Read && s.Into == "" {
		return fmt.Errorf("step %q: read needs Into", s.Name)
	}
	return nil
}

// MissingError lists what the student's data lacks for this flow.
type MissingError struct {
	Fields    []string
	Documents []string
}

func (e *MissingError) Error() string {
	var parts []string
	if len(e.Fields) > 0 {
		parts = append(parts, "missing information: "+strings.Join(e.Fields, ", "))
	}
	if len(e.Documents) > 0 {
		parts = append(parts, "missing documents: "+strings.Join(e.Documents, ", "))
	}
	return strings.Join(parts, "; ")
}

// preflightFields checks only the {{fields}} that steps use.
func preflightFields(steps []Step, b *Bundle) error {
	m := &MissingError{}
	for _, k := range (Flow{Steps: steps}).Fields() {
		if strings.TrimSpace(b.Fields[k]) == "" {
			m.Fields = append(m.Fields, k)
		}
	}
	if len(m.Fields) > 0 {
		return m
	}
	return nil
}

// Preflight checks, before any browser opens, that the bundle has every field
// and document the flow needs. A half-filled application is worse than none.
func Preflight(f Flow, b *Bundle) error {
	m := &MissingError{}
	for _, k := range f.Fields() {
		if strings.TrimSpace(b.Fields[k]) == "" {
			m.Fields = append(m.Fields, k)
		}
	}
	have := map[string]bool{}
	for _, d := range b.Documents {
		have[d.Type] = true
	}
	for _, t := range f.Documents() {
		if !have[t] {
			m.Documents = append(m.Documents, t)
		}
	}
	if len(m.Fields)+len(m.Documents) > 0 {
		return m
	}
	return nil
}

// render replaces {{field}} with the bundle's value.
func render(tmpl string, fields map[string]string) string {
	return fieldRe.ReplaceAllStringFunc(tmpl, func(m string) string {
		return fields[fieldRe.FindStringSubmatch(m)[1]]
	})
}

var (
	ErrNotApproved = errors.New("refusing to submit: the student has not approved this application")
	ErrNeedsHuman  = errors.New("the portal needs the student to complete a check (e.g. CAPTCHA)")
)

// StepError says exactly where a run stopped, with a screenshot.
type StepError struct {
	Index      int
	Step       string
	Err        error
	Screenshot []byte // PNG of the page at the moment of failure
	URL        string
}

func (e *StepError) Error() string {
	return fmt.Sprintf("step %d (%s) failed at %s: %v", e.Index+1, e.Step, e.URL, e.Err)
}

func (e *StepError) Unwrap() error { return e.Err }
