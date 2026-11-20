package portal

// The Agent fills in a portal that has no hand-written Flow. On each page it
// shows a Planner (Claude, in package ai) a snapshot of the page and the
// applicant's approved data, and executes the planned actions in the same
// Chrome engine the Runner uses.
//
// The planner never acts directly. Every action is checked first:
//   - it may only touch fields that are on the current page;
//   - it may only type the applicant's approved values (by key), never text of
//     its own; dropdowns only take an option the page offers;
//   - it may only upload documents from the approved package;
//   - the final submit needs the recorded approval, and never runs in dry run.
// Page content is untrusted (a page could contain instructions aimed at the
// AI); these checks are what keeps a misled planner from doing harm.
//
// Every executed action is also written down as a Flow step, so a run that
// worked can be reviewed and saved as a fixed Flow: the AI maps a portal once,
// the deterministic Runner repeats it cheaply afterwards.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// Option is one choice in a <select>.
type Option struct {
	Value string `json:"value"`
	Text  string `json:"text"`
}

// PageField is one control on the page. Values the page already holds are
// deliberately not included: the planner only needs the structure.
type PageField struct {
	Selector string   `json:"selector"`
	Tag      string   `json:"tag"`
	Type     string   `json:"type,omitempty"`
	Label    string   `json:"label,omitempty"`
	Required bool     `json:"required,omitempty"`
	Options  []Option `json:"options,omitempty"`
}

type PageSnapshot struct {
	URL     string      `json:"url"`
	Title   string      `json:"title"`
	Text    string      `json:"text"`              // visible text, trimmed
	Heading string      `json:"heading,omitempty"` // the page's main heading, used to recognise it later
	Fields  []PageField `json:"fields"`
}

type ActionKind string

const (
	ActFill       ActionKind = "fill"        // type the applicant's value Field into Selector
	ActChoose     ActionKind = "choose"      // pick Option in the <select> at Selector
	ActTick       ActionKind = "tick"        // tick the checkbox/radio at Selector
	ActUpload     ActionKind = "upload"      // attach document DocType
	ActClick      ActionKind = "click"       // press a button (Next, Save, ...), not the final submit
	ActOTP        ActionKind = "otp"         // type the emailed verification code into Selector
	ActSubmit     ActionKind = "submit"      // the final, irreversible submit button
	ActDone       ActionKind = "done"        // the application is submitted; Confirmation is the reference shown
	ActNeedsHuman ActionKind = "needs_human" // something only a person can do (CAPTCHA, payment, missing data)
)

type Action struct {
	Kind         ActionKind `json:"kind"`
	Selector     string     `json:"selector,omitempty"`
	Field        string     `json:"field,omitempty"`
	Option       string     `json:"option,omitempty"`
	DocType      string     `json:"doc_type,omitempty"`
	Confirmation string     `json:"confirmation,omitempty"`
	Reason       string     `json:"reason,omitempty"`
}

type PagePlan struct {
	Actions []Action `json:"actions"`
	Note    string   `json:"note,omitempty"`
}

// PlanInput is everything the planner sees for one page.
type PlanInput struct {
	University string            `json:"university"`
	Program    string            `json:"program"`
	Degree     string            `json:"degree"`
	Fields     map[string]string `json:"applicant_fields"` // approved values; the password is masked
	Documents  []string          `json:"documents"`        // document types that can be uploaded
	History    []string          `json:"history"`          // what was done on earlier pages
	Page       PageSnapshot      `json:"page"`
	Problem    string            `json:"problem,omitempty"` // why the last plan for this page was rejected
}

type Planner interface {
	PlanPage(ctx context.Context, in PlanInput) (*PagePlan, error)
}

// Agent drives one application through an unmapped portal.
type Agent struct {
	Runner   *Runner // browser settings, timeouts, inbox; its step engine executes actions
	Planner  Planner
	MaxPages int // safety limit; default 25
}

// AgentResult is a Result plus the steps that were actually executed.
type AgentResult struct {
	*Result
	Learned Flow
}

const maskedPassword = "(the applicant's portal password; use field portal.password)"

// Run opens startURL and works through the portal page by page.
func (a *Agent) Run(ctx context.Context, startURL string, b *Bundle, opt RunOptions) (*AgentResult, error) {
	if a.Planner == nil {
		return nil, errors.New("agent has no planner")
	}
	if !opt.DryRun && !b.Approved {
		return nil, ErrNotApproved
	}
	maxPages := a.MaxPages
	if maxPages == 0 {
		maxPages = 25
	}
	r := a.Runner
	stepTimeout := r.StepTimeout
	if stepTimeout == 0 {
		stepTimeout = 30 * time.Second
	}
	otpTimeout := r.OTPTimeout
	if otpTimeout == 0 {
		otpTimeout = 5 * time.Minute
	}

	dir, err := os.MkdirTemp("", "applyuni-upload-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	files := map[string]string{}
	var docTypes []string
	for i, d := range b.Documents {
		p := filepath.Join(dir, fmt.Sprintf("%02d-%s", i, safeName(d.Filename)))
		if err := os.WriteFile(p, d.Data, 0o600); err != nil {
			return nil, err
		}
		files[d.Type] = p
		docTypes = append(docTypes, d.Type)
	}
	sort.Strings(docTypes)

	browser, closeBrowser, err := r.startBrowser(ctx)
	if err != nil {
		return nil, err
	}
	defer closeBrowser()

	res := &AgentResult{Result: &Result{Values: map[string]string{}}, Learned: Flow{Portal: b.Portal, Name: b.University + " (learned)"}}
	learn := func(s Step) { res.Learned.Steps = append(res.Learned.Steps, s) }
	learn(Step{Kind: Goto, Name: "Open portal", Value: startURL})
	if err := chromedp.Run(browser, chromedp.Navigate(startURL)); err != nil {
		return res, fmt.Errorf("open %s: %w", startURL, err)
	}

	shown := map[string]string{}
	for k, v := range b.Fields {
		shown[k] = v
	}
	if _, ok := shown["portal.password"]; ok {
		shown["portal.password"] = maskedPassword
	}

	var history []string
	lastPage := ""
	samePage := 0
	for page := 0; page < maxPages; page++ {
		snap, err := snapshot(browser)
		if err != nil {
			return res, fmt.Errorf("read page: %w", err)
		}
		// A page that doesn't change after its actions means we're stuck
		// (a validation error the planner keeps missing, for example).
		key := snap.URL + "\n" + snap.Text
		if key == lastPage {
			samePage++
			if samePage >= 2 {
				return res, a.fail(browser, page, "Page did not change", errors.New("the portal stayed on the same page after our actions; it may be showing an error"))
			}
		} else {
			samePage = 0
		}
		lastPage = key

		in := PlanInput{University: b.University, Program: b.Program, Degree: b.Degree, Fields: shown, Documents: docTypes, History: history, Page: *snap}
		plan, err := a.Planner.PlanPage(ctx, in)
		if err == nil {
			err = checkPlan(plan, snap, b, files)
			if err != nil {
				// One retry with the reason, then give up on this page.
				in.Problem = err.Error()
				if plan, err = a.Planner.PlanPage(ctx, in); err == nil {
					err = checkPlan(plan, snap, b, files)
				}
			}
		}
		if err != nil {
			return res, a.fail(browser, page, "Plan for "+snap.Title, err)
		}

		if snap.Heading != "" {
			learn(Step{Kind: Expect, Name: "On page: " + snap.Heading, Selector: "body", Contains: snap.Heading})
		}
		var did []string
		for _, act := range plan.Actions {
			switch act.Kind {
			case ActNeedsHuman:
				return res, a.fail(browser, page, "Needs a person", fmt.Errorf("%w: %s", ErrNeedsHuman, act.Reason))
			case ActDone:
				res.PortalRef = act.Confirmation
				res.Values["confirmation"] = act.Confirmation
				res.Note = fmt.Sprintf("Submitted through the AI agent in %d pages.", page+1)
				r.receipt(browser, res.Result)
				return res, nil
			case ActSubmit:
				if opt.DryRun {
					res.Note = fmt.Sprintf("Dry run: filled %d pages and stopped before the final submit.", page+1)
					learn(Step{Kind: Submit, Name: "Submit application", Selector: act.Selector})
					r.receipt(browser, res.Result)
					return res, nil
				}
			}
			s := act.step()
			if r.Log != nil {
				r.Log(s.Name)
			}
			timeout := stepTimeout
			if s.Kind == OTP {
				timeout = otpTimeout
			}
			sctx, cancel := context.WithTimeout(browser, timeout)
			err := r.step(sctx, s, b.Fields, files, res.Result, b)
			cancel()
			if err != nil {
				return res, a.fail(browser, page, s.Name, err)
			}
			learn(s)
			did = append(did, s.Name)
			if s.Kind == Submit {
				res.Submitted = true
			}
		}
		history = append(history, fmt.Sprintf("Page %q: %s", snap.Title, strings.Join(did, "; ")))
		waitSettled(browser)
	}
	return res, a.fail(browser, maxPages, "Too many pages", fmt.Errorf("stopped after %d pages without reaching a confirmation", maxPages))
}

func (a *Agent) fail(browser context.Context, page int, name string, err error) error {
	return a.Runner.fail(browser, page, Step{Name: name}, err)
}

// step turns a checked action into an engine step.
func (act Action) step() Step {
	name := act.Reason
	if name == "" {
		name = string(act.Kind) + " " + act.Selector
	}
	switch act.Kind {
	case ActFill:
		return Step{Kind: Fill, Name: name, Selector: act.Selector, Value: "{{" + act.Field + "}}"}
	case ActChoose:
		return Step{Kind: Choose, Name: name, Selector: act.Selector, Value: act.Option}
	case ActTick:
		return Step{Kind: Tick, Name: name, Selector: act.Selector}
	case ActUpload:
		return Step{Kind: Upload, Name: name, Selector: act.Selector, DocType: act.DocType}
	case ActOTP:
		return Step{Kind: OTP, Name: name, Selector: act.Selector, CodeRe: `\b(\d{4,8})\b`}
	case ActSubmit:
		return Step{Kind: Submit, Name: name, Selector: act.Selector}
	default:
		return Step{Kind: Click, Name: name, Selector: act.Selector}
	}
}

// checkPlan is the safety boundary between the planner and the browser.
func checkPlan(p *PagePlan, snap *PageSnapshot, b *Bundle, files map[string]string) error {
	if p == nil || len(p.Actions) == 0 {
		return errors.New("the plan has no actions")
	}
	bySel := map[string]PageField{}
	for _, f := range snap.Fields {
		bySel[f.Selector] = f
	}
	for i, act := range p.Actions {
		last := i == len(p.Actions)-1
		f, onPage := bySel[act.Selector]
		need := func() error {
			if !onPage {
				return fmt.Errorf("action %d (%s): selector %q is not a field on this page", i, act.Kind, act.Selector)
			}
			return nil
		}
		switch act.Kind {
		case ActFill:
			if err := need(); err != nil {
				return err
			}
			if f.Tag == "select" || f.Type == "file" || f.Type == "checkbox" || f.Type == "radio" {
				return fmt.Errorf("action %d: %q is a %s, not a text field", i, act.Selector, f.Type)
			}
			if v, ok := b.Fields[act.Field]; !ok || v == "" {
				return fmt.Errorf("action %d: %q is not one of the applicant's approved values", i, act.Field)
			}
		case ActChoose:
			if err := need(); err != nil {
				return err
			}
			found := false
			for _, o := range f.Options {
				if o.Value == act.Option {
					found = true
				}
			}
			if f.Tag != "select" || !found {
				return fmt.Errorf("action %d: %q is not an option of %q", i, act.Option, act.Selector)
			}
		case ActTick:
			if err := need(); err != nil {
				return err
			}
			if f.Type != "checkbox" && f.Type != "radio" {
				return fmt.Errorf("action %d: %q is not a checkbox", i, act.Selector)
			}
		case ActUpload:
			if err := need(); err != nil {
				return err
			}
			if f.Type != "file" {
				return fmt.Errorf("action %d: %q is not a file input", i, act.Selector)
			}
			if _, ok := files[act.DocType]; !ok {
				return fmt.Errorf("action %d: %q is not in the approved documents", i, act.DocType)
			}
		case ActClick, ActSubmit:
			if err := need(); err != nil {
				return err
			}
			if f.Tag != "button" && f.Type != "submit" && f.Tag != "a" {
				return fmt.Errorf("action %d: %q is not a button or link", i, act.Selector)
			}
			if !last {
				return fmt.Errorf("action %d: a click must be the last action on a page", i)
			}
		case ActOTP:
			if err := need(); err != nil {
				return err
			}
		case ActDone:
			if act.Confirmation == "" || !strings.Contains(snap.Text, act.Confirmation) {
				return fmt.Errorf("action %d: confirmation %q is not shown on the page", i, act.Confirmation)
			}
			if !last {
				return fmt.Errorf("action %d: done must be the last action", i)
			}
		case ActNeedsHuman:
			if act.Reason == "" {
				return fmt.Errorf("action %d: needs_human needs a reason", i)
			}
		default:
			return fmt.Errorf("action %d: unknown kind %q", i, act.Kind)
		}
	}
	return nil
}

// snapshotJS lists visible controls with a stable selector each. It reads
// structure only, never the values typed into the page.
const snapshotJS = `(() => {
  const vis = el => !!(el.offsetWidth || el.offsetHeight || el.getClientRects().length);
  const esc = s => CSS.escape(s);
  const sel = el => {
    if (el.id) return '#' + esc(el.id);
    const tag = el.tagName.toLowerCase();
    if (el.name) {
      const base = tag + '[name="' + el.name + '"]';
      if (el.type === 'radio' || el.type === 'checkbox') return base + '[value="' + el.value + '"]';
      return base;
    }
    // No id or name: a path of tag:nth-of-type steps up to the nearest
    // ancestor with an id (or the body).
    const parts = [];
    for (let n = el; n && n !== document.body; n = n.parentElement) {
      if (n.id) { parts.unshift('#' + esc(n.id)); break; }
      let i = 1;
      for (let s = n.previousElementSibling; s; s = s.previousElementSibling) if (s.tagName === n.tagName) i++;
      parts.unshift(n.tagName.toLowerCase() + ':nth-of-type(' + i + ')');
    }
    if (!parts.length || !parts[0].startsWith('#')) parts.unshift('body');
    return parts.join(' > ');
  };
  const label = el => {
    if (el.id) { const l = document.querySelector('label[for="' + esc(el.id) + '"]'); if (l) return l.innerText; }
    const p = el.closest('label'); if (p) return p.innerText;
    if (el.getAttribute('aria-label')) return el.getAttribute('aria-label');
    if (el.placeholder) return el.placeholder;
    if (el.tagName === 'BUTTON' || el.tagName === 'A') return el.innerText;
    if (el.type === 'submit') return el.value;
    return '';
  };
  const fields = [];
  for (const el of document.querySelectorAll('input, select, textarea, button, a[href]')) {
    if (el.type === 'hidden' || (!vis(el) && el.type !== 'file')) continue;
    if (el.tagName === 'A' && !el.id && !/button|btn/i.test(el.className + ' ' + (el.getAttribute('role') || ''))) continue;
    const s = sel(el); if (!s) continue;
    fields.push({
      selector: s, tag: el.tagName.toLowerCase(), type: el.type || '',
      label: label(el).replace(/\s+/g, ' ').trim().slice(0, 160),
      required: !!el.required,
      options: el.tagName === 'SELECT' ? [...el.options].slice(0, 300).map(o => ({value: o.value, text: o.text.trim()})) : undefined,
    });
  }
  const h = [...document.querySelectorAll('h1, h2, legend')].find(vis);
  return { url: location.href, title: document.title, heading: h ? h.innerText.replace(/\s+/g, ' ').trim().slice(0, 120) : '', text: document.body.innerText.replace(/\s+/g, ' ').trim().slice(0, 6000), fields };
})()`

func snapshot(ctx context.Context) (*PageSnapshot, error) {
	sctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var raw json.RawMessage
	if err := chromedp.Run(sctx, chromedp.WaitReady("body", chromedp.ByQuery), chromedp.Evaluate(snapshotJS, &raw)); err != nil {
		return nil, err
	}
	var s PageSnapshot
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// waitSettled gives a navigation started by a click time to finish.
func waitSettled(ctx context.Context) {
	sctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	time.Sleep(300 * time.Millisecond)
	_ = chromedp.Run(sctx, chromedp.WaitReady("body", chromedp.ByQuery))
}
