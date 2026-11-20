package portal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"
)

// ---------------------------------------------------------------------------
// A fake university portal: register → email code → personal → education →
// documents → review → submit → confirmation. It behaves like a real one
// (sessions, redirects, multipart uploads) so the engine is tested for real.
// ---------------------------------------------------------------------------

type fakePortal struct {
	mu        sync.Mutex
	inbox     *MemoryInbox
	renamed   bool // simulate a portal redesign: the GPA field gets a new id
	accounts  map[string]string
	codes     map[string]string
	form      map[string]string
	files     map[string][]byte
	submitted int
}

func newFakePortal(inbox *MemoryInbox) (*fakePortal, *httptest.Server) {
	fp := &fakePortal{inbox: inbox, accounts: map[string]string{}, codes: map[string]string{}, form: map[string]string{}, files: map[string][]byte{}}
	return fp, httptest.NewServer(fp)
}

func page(w http.ResponseWriter, title, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!doctype html><html><head><title>%s</title></head><body><h1 id="title">%s</h1>%s</body></html>`, title, title, body)
}

func (fp *fakePortal) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fp.mu.Lock()
	defer fp.mu.Unlock()
	user := ""
	if c, err := r.Cookie("session"); err == nil {
		user = c.Value
	}
	switch r.Method + " " + r.URL.Path {
	case "GET /register":
		page(w, "Create your account", `<form method="post" action="/register">
			<input id="email" name="email" type="email"><input id="password" name="password" type="password">
			<button id="create" type="submit">Create account</button></form>`)
	case "POST /register":
		r.ParseForm()
		email := r.FormValue("email")
		fp.accounts[email] = r.FormValue("password")
		fp.codes[email] = "482913"
		go fp.inbox.Deliver(email, "Welcome to the portal. Your verification code is 482913. It expires in 10 minutes.")
		http.SetCookie(w, &http.Cookie{Name: "pending", Value: email, Path: "/"})
		http.Redirect(w, r, "/verify", http.StatusSeeOther)
	case "GET /verify":
		page(w, "Verify your email", `<form method="post" action="/verify"><input id="code" name="code"><button id="verify" type="submit">Verify</button></form>`)
	case "POST /verify":
		r.ParseForm()
		c, _ := r.Cookie("pending")
		if c == nil || fp.codes[c.Value] != r.FormValue("code") {
			page(w, "Wrong code", "")
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "session", Value: c.Value, Path: "/"})
		http.Redirect(w, r, "/apply/personal", http.StatusSeeOther)
	case "GET /apply/personal":
		page(w, "Personal details", `<form method="post" action="/apply/personal">
			<input id="first_name" name="first_name"><input id="last_name" name="last_name">
			<select id="citizenship" name="citizenship"><option value="">Choose</option><option value="UZ">Uzbekistan</option><option value="IT">Italy</option></select>
			<button id="next" type="submit">Next</button></form>`)
	case "GET /apply/education":
		gpa := "gpa"
		if fp.renamed {
			gpa = "grade_average"
		}
		page(w, "Education", fmt.Sprintf(`<form method="post" action="/apply/education">
			<input id="university" name="university"><input id="%s" name="gpa">
			<label><input id="declaration" name="declaration" type="checkbox" value="yes"> I declare the information is true</label>
			<button id="next" type="submit">Next</button></form>`, gpa))
	case "POST /apply/personal", "POST /apply/education":
		if user == "" {
			http.Error(w, "not logged in", 401)
			return
		}
		r.ParseForm()
		for k, v := range r.PostForm {
			fp.form[k] = v[0]
		}
		next := map[string]string{"/apply/personal": "/apply/education", "/apply/education": "/apply/documents"}[r.URL.Path]
		http.Redirect(w, r, next, http.StatusSeeOther)
	case "GET /apply/documents":
		page(w, "Documents", `<form method="post" action="/apply/documents" enctype="multipart/form-data">
			<input id="passport" name="passport" type="file"><input id="transcript" name="transcript" type="file">
			<button id="next" type="submit">Upload</button></form>`)
	case "POST /apply/documents":
		r.ParseMultipartForm(10 << 20)
		for _, k := range []string{"passport", "transcript"} {
			if f, _, err := r.FormFile(k); err == nil {
				b, _ := io.ReadAll(f)
				fp.files[k] = b
			}
		}
		http.Redirect(w, r, "/review", http.StatusSeeOther)
	case "GET /review":
		page(w, "Review your application", fmt.Sprintf(`<p id="summary">%s %s, %s</p>
			<form method="post" action="/submit"><button id="submit-application" type="submit">Submit application</button></form>`,
			html.EscapeString(fp.form["first_name"]), html.EscapeString(fp.form["last_name"]), html.EscapeString(fp.form["university"])))
	case "POST /submit":
		fp.submitted++
		http.Redirect(w, r, "/done", http.StatusSeeOther)
	case "GET /done":
		page(w, "Application submitted", `<p>Your application number is <span id="app-number">APP-2027-0042</span>.</p>`)
	default:
		http.NotFound(w, r)
	}
}

// The recipe for the fake portal, written exactly as a real one would be.
func fakeFlow(base string) Flow {
	return Flow{Portal: "fake", Name: "Fake University", Steps: []Step{
		{Kind: Goto, Name: "Open registration", Value: base + "/register"},
		{Kind: Expect, Name: "On registration page", Selector: "#title", Contains: "Create your account"},
		{Kind: Fill, Name: "Email", Selector: "#email", Value: "{{student.email}}"},
		{Kind: Fill, Name: "Password", Selector: "#password", Value: "{{portal.password}}"},
		{Kind: Click, Name: "Create account", Selector: "#create"},
		{Kind: OTP, Name: "Email verification code", Selector: "#code", CodeRe: `code is (\d{6})`},
		{Kind: Click, Name: "Verify", Selector: "#verify"},
		{Kind: Expect, Name: "On personal details", Selector: "#title", Contains: "Personal details"},
		{Kind: Fill, Name: "First name", Selector: "#first_name", Value: "{{student.first_name}}"},
		{Kind: Fill, Name: "Last name", Selector: "#last_name", Value: "{{student.last_name}}"},
		{Kind: Choose, Name: "Citizenship", Selector: "#citizenship", Value: "UZ"},
		{Kind: Click, Name: "Next (personal)", Selector: "#next"},
		{Kind: Expect, Name: "On education", Selector: "#title", Contains: "Education"},
		{Kind: Fill, Name: "University", Selector: "#university", Value: "{{profile.university}}"},
		{Kind: Fill, Name: "GPA", Selector: "#gpa", Value: "{{profile.gpa}}"},
		{Kind: Tick, Name: "Declaration", Selector: "#declaration"},
		{Kind: Click, Name: "Next (education)", Selector: "#next"},
		{Kind: Expect, Name: "On documents", Selector: "#title", Contains: "Documents"},
		{Kind: Upload, Name: "Passport", Selector: "#passport", DocType: "passport"},
		{Kind: Upload, Name: "Transcript", Selector: "#transcript", DocType: "bachelor_transcript"},
		{Kind: Click, Name: "Upload documents", Selector: "#next"},
		{Kind: Expect, Name: "On review", Selector: "#summary", Contains: "Karimova"},
		{Kind: Submit, Name: "Submit application", Selector: "#submit-application"},
		{Kind: Expect, Name: "Confirmation page", Selector: "#title", Contains: "submitted"},
		{Kind: Read, Name: "Application number", Selector: "#app-number", Into: "confirmation"},
	}}
}

var (
	passportPDF   = []byte("%PDF-1.4 passport scan of Nilufar Karimova")
	transcriptPDF = []byte("%PDF-1.4 transcript: 3.4/4")
)

func bundle(approved bool) *Bundle {
	return &Bundle{
		Portal: "fake", Approved: approved,
		Fields: BuildFields("Nilufar Karimova", "s-abc123@inbox.applyuni.local", "s-abc123@inbox.applyuni.local", "Pw-Strong-123!",
			map[string]any{"university": "Westminster International University in Tashkent", "gpa": "3.4/4"}, "Computer Science", "master"),
		Documents: []Document{
			{Type: "passport", Filename: "passport.pdf", ContentType: "application/pdf", Data: passportPDF},
			{Type: "bachelor_transcript", Filename: "transcript.pdf", ContentType: "application/pdf", Data: transcriptPDF},
		},
	}
}

func newRunner(t *testing.T, inbox Inbox) *Runner {
	t.Helper()
	path := ""
	for _, c := range []string{"google-chrome", "chromium", "chromium-browser"} {
		if p, err := exec.LookPath(c); err == nil {
			path = p
			break
		}
	}
	if path == "" {
		t.Skip("Chrome not installed")
	}
	return &Runner{ChromePath: path, Headless: true, StepTimeout: 15 * time.Second, OTPTimeout: 15 * time.Second, Inbox: inbox}
}

// ---------------------------------------------------------------------------

func TestFullApprovedSubmission(t *testing.T) {
	inbox := NewMemoryInbox()
	fp, srv := newFakePortal(inbox)
	defer srv.Close()
	var log []string
	r := newRunner(t, inbox)
	r.Log = func(s string) { log = append(log, s) }

	res, err := r.Run(context.Background(), fakeFlow(srv.URL), bundle(true), RunOptions{})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if !res.Submitted || fp.submitted != 1 {
		t.Fatalf("expected exactly one submission, got %d (submitted=%v)", fp.submitted, res.Submitted)
	}
	if res.PortalRef != "APP-2027-0042" {
		t.Errorf("confirmation number: %q", res.PortalRef)
	}
	if len(res.Receipt) < 1000 || !bytes.HasPrefix(res.Receipt, []byte("\x89PNG")) {
		t.Errorf("receipt screenshot missing (%d bytes)", len(res.Receipt))
	}
	if fp.accounts["s-abc123@inbox.applyuni.local"] != "Pw-Strong-123!" {
		t.Error("account not created with the generated password")
	}
	want := map[string]string{"first_name": "Nilufar", "last_name": "Karimova", "citizenship": "UZ",
		"university": "Westminster International University in Tashkent", "gpa": "3.4/4", "declaration": "yes"}
	for k, v := range want {
		if fp.form[k] != v {
			t.Errorf("portal received %s=%q, want %q", k, fp.form[k], v)
		}
	}
	if !bytes.Equal(fp.files["passport"], passportPDF) || !bytes.Equal(fp.files["transcript"], transcriptPDF) {
		t.Error("uploaded files differ from the student's documents")
	}
	if len(log) != len(fakeFlow(srv.URL).Steps) {
		t.Errorf("progress log has %d entries", len(log))
	}
}

func TestDryRunFillsEverythingButNeverSubmits(t *testing.T) {
	inbox := NewMemoryInbox()
	fp, srv := newFakePortal(inbox)
	defer srv.Close()
	res, err := newRunner(t, inbox).Run(context.Background(), fakeFlow(srv.URL), bundle(false), RunOptions{DryRun: true})
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	if fp.submitted != 0 || res.Submitted {
		t.Fatal("dry run must never submit")
	}
	if fp.form["gpa"] != "3.4/4" || len(fp.files) != 2 {
		t.Error("dry run should still fill in and upload everything up to the review page")
	}
	if !strings.Contains(res.Note, "Dry run") || len(res.Receipt) == 0 {
		t.Errorf("dry run should explain itself and screenshot the review page: %q", res.Note)
	}
}

func TestRefusesWithoutApproval(t *testing.T) {
	inbox := NewMemoryInbox()
	fp, srv := newFakePortal(inbox)
	defer srv.Close()
	_, err := newRunner(t, inbox).Run(context.Background(), fakeFlow(srv.URL), bundle(false), RunOptions{})
	if !errors.Is(err, ErrNotApproved) {
		t.Fatalf("expected ErrNotApproved, got %v", err)
	}
	if len(fp.accounts) != 0 || fp.submitted != 0 {
		t.Fatal("nothing may reach the portal without approval")
	}
}

func TestPreflightStopsBeforeTheBrowser(t *testing.T) {
	inbox := NewMemoryInbox()
	fp, srv := newFakePortal(inbox)
	defer srv.Close()
	b := bundle(true)
	delete(b.Fields, "profile.gpa")
	b.Documents = b.Documents[:1] // no transcript
	_, err := newRunner(t, inbox).Run(context.Background(), fakeFlow(srv.URL), b, RunOptions{})
	var m *MissingError
	if !errors.As(err, &m) || len(m.Fields) != 1 || m.Fields[0] != "profile.gpa" || len(m.Documents) != 1 || m.Documents[0] != "bachelor_transcript" {
		t.Fatalf("expected missing gpa + transcript, got %v", err)
	}
	if len(fp.accounts) != 0 {
		t.Fatal("preflight failures must not touch the portal")
	}
}

func TestPortalChangeIsReportedWithScreenshot(t *testing.T) {
	inbox := NewMemoryInbox()
	fp, srv := newFakePortal(inbox)
	defer srv.Close()
	fp.renamed = true // the portal renamed its GPA field
	r := newRunner(t, inbox)
	r.StepTimeout = 3 * time.Second
	_, err := r.Run(context.Background(), fakeFlow(srv.URL), bundle(true), RunOptions{})
	var se *StepError
	if !errors.As(err, &se) {
		t.Fatalf("expected a StepError, got %v", err)
	}
	if se.Step != "GPA" || !strings.HasSuffix(se.URL, "/apply/education") || len(se.Screenshot) == 0 {
		t.Errorf("error should name the step, page and include a screenshot: step=%q url=%q shot=%d", se.Step, se.URL, len(se.Screenshot))
	}
	if fp.submitted != 0 {
		t.Fatal("a broken run must not submit")
	}
}

func TestHumanStepPauses(t *testing.T) {
	inbox := NewMemoryInbox()
	_, srv := newFakePortal(inbox)
	defer srv.Close()
	f := Flow{Portal: "fake", Steps: []Step{
		{Kind: Goto, Name: "Open", Value: srv.URL + "/register"},
		{Kind: Human, Name: "CAPTCHA"},
		{Kind: Submit, Name: "Submit", Selector: "#create"},
	}}
	_, err := newRunner(t, inbox).Run(context.Background(), f, bundle(true), RunOptions{})
	if !errors.Is(err, ErrNeedsHuman) {
		t.Fatalf("expected ErrNeedsHuman, got %v", err)
	}
}

func TestFlowValidation(t *testing.T) {
	ok := fakeFlow("http://x")
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid flow rejected: %v", err)
	}
	bad := map[string]Flow{
		"no submit":  {Portal: "x", Steps: []Step{{Kind: Goto, Name: "a", Value: "http://x"}}},
		"two submit": {Portal: "x", Steps: []Step{{Kind: Submit, Name: "a", Selector: "#a"}, {Kind: Submit, Name: "b", Selector: "#b"}}},
		"bad otp":    {Portal: "x", Steps: []Step{{Kind: OTP, Name: "a", Selector: "#c", CodeRe: `\d+`}, {Kind: Submit, Name: "s", Selector: "#s"}}},
		"no name":    {Portal: "x", Steps: []Step{{Kind: Submit, Selector: "#s"}}},
		"upload doc": {Portal: "x", Steps: []Step{{Kind: Upload, Name: "u", Selector: "#f"}, {Kind: Submit, Name: "s", Selector: "#s"}}},
	}
	for name, f := range bad {
		if f.Validate() == nil {
			t.Errorf("%s: should be rejected", name)
		}
	}
	if got := ok.Fields(); strings.Join(got, ",") != "portal.password,profile.gpa,profile.university,student.email,student.first_name,student.last_name" {
		t.Errorf("fields: %v", got)
	}
}

func TestPortalPassword(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		pw := NewPortalPassword()
		var up, lo, di, sy bool
		for _, c := range pw {
			switch {
			case unicode.IsUpper(c):
				up = true
			case unicode.IsLower(c):
				lo = true
			case unicode.IsDigit(c):
				di = true
			default:
				sy = true
			}
		}
		if len(pw) != 20 || !up || !lo || !di || !sy || seen[pw] {
			t.Fatalf("weak or repeated password %q", pw)
		}
		seen[pw] = true
	}
}
