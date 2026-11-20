package portal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// Runner executes a Flow in a real (headless) Chrome.
type Runner struct {
	ChromePath  string        // empty = let chromedp find Chrome
	Headless    bool          // false only when debugging a new flow
	StepTimeout time.Duration // per step; default 30s
	OTPTimeout  time.Duration // wait for verification emails; default 5 min
	Inbox       Inbox
	Log         func(step string) // optional progress callback
}

// RunOptions control a single run.
type RunOptions struct {
	DryRun bool // do everything except the Submit step
}

// Run fills in and (if approved and not dry-run) submits the application.
// It always returns what it collected so far (Read values, receipt).
func (r *Runner) Run(ctx context.Context, f Flow, b *Bundle, opt RunOptions) (*Result, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	if err := Preflight(f, b); err != nil {
		return nil, err
	}
	if !opt.DryRun && !b.Approved {
		return nil, ErrNotApproved
	}
	return r.run(ctx, f.Steps, b, opt)
}

// CreateAccount runs the flow's Account steps: register on the portal with
// the student's platform email and generated password, then confirm the
// emailed code. It needs the student's approval like a submission does,
// because it hands their personal data to the university.
func (r *Runner) CreateAccount(ctx context.Context, f Flow, b *Bundle) (*Result, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	if len(f.Account) == 0 {
		return &Result{Note: "This portal has no separate registration."}, nil
	}
	if !b.Approved {
		return nil, ErrNotApproved
	}
	if err := preflightFields(f.Account, b); err != nil {
		return nil, err
	}
	return r.run(ctx, f.Account, b, RunOptions{})
}

func (r *Runner) run(ctx context.Context, steps []Step, b *Bundle, opt RunOptions) (*Result, error) {
	stepTimeout := r.StepTimeout
	if stepTimeout == 0 {
		stepTimeout = 30 * time.Second
	}
	otpTimeout := r.OTPTimeout
	if otpTimeout == 0 {
		otpTimeout = 5 * time.Minute
	}

	// Documents are written to a private temp dir only for the upload, then removed.
	dir, err := os.MkdirTemp("", "applyuni-upload-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	files := map[string]string{}
	for i, d := range b.Documents {
		name := fmt.Sprintf("%02d-%s", i, safeName(d.Filename))
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, d.Data, 0o600); err != nil {
			return nil, err
		}
		files[d.Type] = p
	}

	browser, closeBrowser, err := r.startBrowser(ctx)
	if err != nil {
		return nil, err
	}
	defer closeBrowser()

	res := &Result{Values: map[string]string{}}
	fields := b.Fields

	for i, s := range steps {
		if r.Log != nil {
			r.Log(s.Name)
		}
		if s.Kind == Submit && opt.DryRun {
			res.Note = "Dry run: stopped before the final submit."
			break
		}
		if s.Kind == Human {
			return res, r.fail(browser, i, s, ErrNeedsHuman)
		}

		sctx, cancel := context.WithTimeout(browser, stepTimeout)
		if s.Kind == OTP {
			cancel()
			sctx, cancel = context.WithTimeout(browser, otpTimeout)
		}
		err := r.step(sctx, s, fields, files, res, b)
		cancel()
		if err != nil {
			if s.Optional && errors.Is(err, context.DeadlineExceeded) {
				continue
			}
			return res, r.fail(browser, i, s, err)
		}
		if s.Kind == Submit {
			res.Submitted = true
		}
	}

	r.receipt(browser, res)
	if ref, ok := res.Values["confirmation"]; ok {
		res.PortalRef = ref
	}
	return res, nil
}

// startBrowser launches Chrome. The browser lives on ctx: chromedp ties its
// lifetime to the context of the first Run, so starting it inside a per-step
// timeout would close it when that step finishes.
func (r *Runner) startBrowser(ctx context.Context) (context.Context, func(), error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", r.Headless),
		chromedp.Flag("disable-gpu", true),
		chromedp.WindowSize(1280, 1000),
	)
	if r.ChromePath != "" {
		opts = append(opts, chromedp.ExecPath(r.ChromePath))
	}
	alloc, cancelAlloc := chromedp.NewExecAllocator(ctx, opts...)
	browser, cancelBrowser := chromedp.NewContext(alloc)
	closeAll := func() { cancelBrowser(); cancelAlloc() }
	if err := chromedp.Run(browser); err != nil {
		closeAll()
		return nil, nil, fmt.Errorf("start browser: %w", err)
	}
	return browser, closeAll, nil
}

// receipt saves a PNG screenshot of the current page (quality 100 = PNG;
// below that chromedp makes JPEG).
func (r *Runner) receipt(browser context.Context, res *Result) {
	var shot []byte
	if err := chromedp.Run(browser, chromedp.FullScreenshot(&shot, 100)); err == nil {
		res.Receipt = shot
	}
}

func (r *Runner) step(ctx context.Context, s Step, fields, files map[string]string, res *Result, b *Bundle) error {
	q := chromedp.ByQuery
	switch s.Kind {
	case Goto:
		return chromedp.Run(ctx, chromedp.Navigate(render(s.Value, fields)))
	case Expect:
		// Pages change after a click with some delay, so keep checking until
		// the expected page shows up or the step times out.
		var text string
		for {
			err := chromedp.Run(ctx, chromedp.WaitVisible(s.Selector, q), chromedp.Text(s.Selector, &text, q))
			if err == nil && (s.Contains == "" || strings.Contains(text, s.Contains)) {
				return nil
			}
			select {
			case <-ctx.Done():
				if err != nil {
					return fmt.Errorf("expected page element %q not found (the portal may have changed): %w", s.Selector, err)
				}
				return fmt.Errorf("expected %q to contain %q, got %q (the portal may have changed)", s.Selector, s.Contains, text)
			case <-time.After(250 * time.Millisecond):
			}
		}
	case Fill:
		return chromedp.Run(ctx, chromedp.WaitVisible(s.Selector, q), chromedp.Clear(s.Selector, q),
			chromedp.SendKeys(s.Selector, render(s.Value, fields), q))
	case Choose:
		return chromedp.Run(ctx, chromedp.WaitVisible(s.Selector, q), chromedp.SetValue(s.Selector, render(s.Value, fields), q))
	case Tick:
		return chromedp.Run(ctx, chromedp.WaitVisible(s.Selector, q), chromedp.Click(s.Selector, q))
	case Upload:
		p, ok := files[s.DocType]
		if !ok {
			return fmt.Errorf("no %s document in the bundle", s.DocType)
		}
		return chromedp.Run(ctx, chromedp.WaitReady(s.Selector, q), chromedp.SetUploadFiles(s.Selector, []string{p}, q))
	case Click:
		return chromedp.Run(ctx, chromedp.WaitVisible(s.Selector, q), chromedp.Click(s.Selector, q))
	case OTP:
		if r.Inbox == nil {
			return errors.New("no inbox configured for verification codes")
		}
		since := time.Now().Add(-2 * time.Minute) // the email may arrive just before we start waiting
		code, err := r.Inbox.WaitFor(ctx, b.Fields["student.email"], since, regexp.MustCompile(s.CodeRe))
		if err != nil {
			return err
		}
		return chromedp.Run(ctx, chromedp.WaitVisible(s.Selector, q), chromedp.SendKeys(s.Selector, code, q))
	case Read:
		var text string
		if err := chromedp.Run(ctx, chromedp.WaitVisible(s.Selector, q), chromedp.Text(s.Selector, &text, q)); err != nil {
			return err
		}
		res.Values[s.Into] = strings.TrimSpace(text)
		return nil
	case Submit:
		if !b.Approved {
			return ErrNotApproved // second guard, right at the irreversible click
		}
		return chromedp.Run(ctx, chromedp.WaitVisible(s.Selector, q), chromedp.Click(s.Selector, q))
	}
	return fmt.Errorf("unknown step kind %q", s.Kind)
}

// fail captures where we stopped, with a screenshot, without letting a
// screenshot problem hide the real error.
func (r *Runner) fail(browser context.Context, i int, s Step, err error) error {
	e := &StepError{Index: i, Step: s.Name, Err: err}
	ctx, cancel := context.WithTimeout(browser, 10*time.Second)
	defer cancel()
	_ = chromedp.Run(ctx, chromedp.Location(&e.URL), chromedp.FullScreenshot(&e.Screenshot, 100))
	return e
}

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func safeName(s string) string {
	s = unsafeChars.ReplaceAllString(filepath.Base(s), "_")
	if s == "" || s == "." {
		return "document"
	}
	return s
}

// FlowAdapter plugs a Flow into the Registry.
type FlowAdapter struct {
	Flow   Flow
	Runner *Runner
	DryRun bool
}

func (a FlowAdapter) Submit(ctx context.Context, b *Bundle) (*Result, error) {
	return a.Runner.Run(ctx, a.Flow, b, RunOptions{DryRun: a.DryRun})
}

func (a FlowAdapter) CreateAccount(ctx context.Context, b *Bundle) (*Result, error) {
	return a.Runner.CreateAccount(ctx, a.Flow, b)
}
