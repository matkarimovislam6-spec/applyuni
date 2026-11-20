package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/matkarimovislam6-spec/applyuni/backend/internal/auth"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/db"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/portal"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/verify"
)

// fakeAI reads documents predictably: what each "document" says is in its
// bytes, as JSON after the PDF header.
type fakeAI struct{}

func (fakeAI) AnalyzeDocument(_ context.Context, expected, _ string, data []byte) (*verify.Extraction, json.RawMessage, error) {
	body := string(data)
	body = body[strings.Index(body, "{") : strings.LastIndex(body, "}")+1]
	e := verify.Extraction{MatchesExpected: true, Legible: true, Confidence: 0.95, Accept: true, Fields: map[string]string{}}
	if err := json.Unmarshal([]byte(body), &e.Fields); err != nil {
		return nil, nil, err
	}
	if t := e.Fields["is"]; t != "" && t != expected {
		e.MatchesExpected, e.DetectedType = false, t
	}
	raw, _ := json.Marshal(e)
	return &e, raw, nil
}

func doc(fields string) []byte { return []byte("%PDF-1.4\n" + fields + "\n%%EOF\n") }

// student signs up, fills a master's profile and starts one application.
func newApplicant(t *testing.T, s *Service) (*auth.Claims, uuid.UUID, *Application) {
	t.Helper()
	ctx := context.Background()
	sess, err := s.Register(ctx, RegisterInput{Username: fmt.Sprintf("st%d", rand.Intn(1e9)), Password: "supersecret1", FullName: "Islom Matkarimov"})
	if err != nil {
		t.Fatal(err)
	}
	c := claimsOf(t, s, sess)
	me, _ := s.Me(ctx, c)
	sid := *me.StudentID
	if _, err := s.UpdateProfile(ctx, c, sid, []byte(`{"degree_target":"master","education_level":"bachelor_done","gpa":"3.6/4","documents_language":"en"}`)); err != nil {
		t.Fatal(err)
	}
	progs, _ := s.ListPrograms(ctx, "IT", "master")
	app, err := s.CreateApplication(ctx, c, sid, progs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	return c, sid, app
}

// Stages 1+2: intake, AI reading, rules and the cross-document check.
func TestDocumentsAreCheckedAgainstEachOther(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestService(t)
	s.AI = fakeAI{}
	c, sid, app := newApplicant(t, s)

	upload := func(docType string, fields string) *Document {
		t.Helper()
		d, err := s.UploadDocument(ctx, c, sid, docType, docType+".pdf", "text/plain", doc(fields))
		if err != nil {
			t.Fatalf("upload %s: %v", docType, err)
		}
		drain(t, s)
		d, _ = getDocFor(t, s, c, d.ID)
		return d
	}

	pass := upload("passport", `{"full_name":"MATKARIMOV ISLOM","date_of_birth":"2004-03-01","document_number":"FA1234567","nationality":"UZB","expiry_date":"2032-01-01"}`)
	if pass.Status != "accepted" {
		t.Fatalf("passport: %s %s", pass.Status, pass.Verdict)
	}
	// The browser's content type is ignored: the bytes say PDF.
	if pass.ContentType != "application/pdf" {
		t.Errorf("content type from bytes: %s", pass.ContentType)
	}
	// Exact re-upload: the same document comes back, nothing new queued.
	again, _ := s.UploadDocument(ctx, c, sid, "passport", "passport (1).pdf", "", doc(`{"full_name":"MATKARIMOV ISLOM","date_of_birth":"2004-03-01","document_number":"FA1234567","nationality":"UZB","expiry_date":"2032-01-01"}`))
	if again.ID != pass.ID {
		t.Error("duplicate upload should return the existing document")
	}

	// Someone else's diploma: held for a person, application not ready.
	wrong := upload("bachelor_diploma", `{"full_name":"Aziz Karimov","institution":"TUIT","degree_name":"BSc","issue_date":"2025-06-30"}`)
	if wrong.Status != "needs_review" || !strings.Contains(string(wrong.Verdict), "doesn't match your passport") {
		t.Fatalf("mismatched name should need review: %s %s", wrong.Status, wrong.Verdict)
	}
	// Wrong document type: rejected with a reason.
	bad := upload("bachelor_transcript", `{"is":"bank_statement","full_name":"Islom Matkarimov"}`)
	if bad.Status != "rejected" || !strings.Contains(string(bad.Verdict), "bank statement") {
		t.Fatalf("wrong type should be rejected: %s %s", bad.Status, bad.Verdict)
	}

	// The student uploads the right files: Cyrillic name on the diploma still matches.
	right := upload("bachelor_diploma", `{"full_name":"Маткаримов Ислом Баходирович","institution":"TUIT","degree_name":"BSc","issue_date":"2025-06-30"}`)
	if right.Status != "accepted" {
		t.Fatalf("Cyrillic diploma should be accepted: %s %s", right.Status, right.Verdict)
	}
	var oldStatus string
	s.DB.Q().QueryRow(ctx, `SELECT status FROM documents WHERE id = ?`, wrong.ID).Scan(&oldStatus)
	if oldStatus != "superseded" {
		t.Errorf("old diploma should be superseded, is %s", oldStatus)
	}
	upload("bachelor_transcript", `{"full_name":"Islom Matkarimov","institution":"TUIT","grade_average":"3.6","grade_scale":"4"}`)

	// Upload whatever else the checklist asks for, then the application is ready.
	d, _ := s.GetApplication(ctx, c, app.ID)
	for _, it := range d.Checklist.Items {
		if it.Status == "missing" {
			upload(it.Type, `{"full_name":"Islom Matkarimov","institution":"x","degree_name":"x","issue_date":"2026-01-01","grade_average":"3","grade_scale":"4","test_name":"IELTS","overall_score":"7","test_date":"2026-06-01","date_of_birth":"2004-03-01","translator":"x","recommender":"x","balance":"1","currency":"EUR"}`)
		}
	}
	d, _ = s.GetApplication(ctx, c, app.ID)
	if d.Status != "ready_for_review" {
		for _, it := range d.Checklist.Items {
			t.Logf("%s: %s", it.Type, it.Status)
		}
		t.Fatalf("expected ready_for_review, got %s", d.Status)
	}
}

func getDocFor(t *testing.T, s *Service, c *auth.Claims, id uuid.UUID) (*Document, error) {
	t.Helper()
	docs, err := s.ListDocuments(context.Background(), c, *mustStudent(t, s, c))
	if err != nil {
		return nil, err
	}
	for _, d := range docs {
		if d.ID == id {
			return d, nil
		}
	}
	var d *Document
	err = s.DB.Tenant(context.Background(), c.OrgID, func(tx db.Conn) error {
		var e error
		d, e = getDoc(context.Background(), tx, c.OrgID, id)
		return e
	})
	return d, err
}

func mustStudent(t *testing.T, s *Service, c *auth.Claims) *uuid.UUID {
	me, err := s.Me(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	return me.StudentID
}

// A portal with real registration: email + password, a 6-digit code sent by
// email, then login, the form, upload and submit.
type uniPortal struct {
	mu        sync.Mutex
	s         *Service
	t         *testing.T
	accounts  map[string]string // email -> password
	codes     map[string]string
	active    map[string]bool
	submitted int
	got       map[string]string
}

func (u *uniPortal) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	page := func(title, body string) { fmt.Fprintf(w, `<h1 id="t">%s</h1>%s`, title, body) }
	r.ParseMultipartForm(5 << 20)
	switch r.Method + " " + r.URL.Path {
	case "GET /register":
		page("Create your account", `<form method="post"><input id="email" name="email"><input id="pw" name="pw" type="password"><button id="create">Create</button></form>`)
	case "POST /register":
		email, code := r.FormValue("email"), fmt.Sprintf("%06d", rand.Intn(1e6))
		u.mu.Lock()
		u.accounts[email], u.codes[email] = r.FormValue("pw"), code
		u.mu.Unlock()
		// The portal emails the code; it reaches us through the inbound webhook path.
		go func() {
			time.Sleep(300 * time.Millisecond)
			if err := u.s.ReceiveEmail(context.Background(), InboundEmail{To: "Student <" + email + ">", From: "Uni Portal <noreply@apply.unitest.it>",
				Subject: "Your verification code", Text: "Hello, your verification code is " + code + ". It expires in 10 minutes."}); err != nil {
				u.t.Errorf("deliver code: %v", err)
			}
		}()
		http.Redirect(w, r, "/verify?email="+email, http.StatusSeeOther)
	case "GET /verify":
		page("Check your email", `<form method="post" action="/verify?email=`+r.URL.Query().Get("email")+`"><input id="code" name="code"><button id="ok">Verify</button></form>`)
	case "POST /verify":
		email := r.URL.Query().Get("email")
		u.mu.Lock()
		okCode := r.FormValue("code") == u.codes[email]
		if okCode {
			u.active[email] = true
		}
		u.mu.Unlock()
		if !okCode {
			page("Wrong code", "")
			return
		}
		page("Account ready", "")
	case "GET /login":
		page("Log in", `<form method="post"><input id="email" name="email"><input id="pw" name="pw" type="password"><button id="go">Log in</button></form>`)
	case "POST /login":
		u.mu.Lock()
		ok := u.active[r.FormValue("email")] && u.accounts[r.FormValue("email")] == r.FormValue("pw")
		u.mu.Unlock()
		if !ok {
			page("Login failed", "")
			return
		}
		page("Application form", `<form method="post" action="/apply" enctype="multipart/form-data"><input id="name" name="name"><input id="gpa" name="gpa"><input id="passport" name="passport" type="file"><button id="send">Submit</button></form>`)
	case "POST /apply":
		u.mu.Lock()
		u.submitted++
		u.got = map[string]string{"name": r.FormValue("name"), "gpa": r.FormValue("gpa")}
		u.mu.Unlock()
		page("Application submitted", `<p>Number <b id="ref">UT-2027-0099</b></p>`)
	default:
		http.NotFound(w, r)
	}
}

func uniFlow(base string) portal.Flow {
	return portal.Flow{Portal: "unitest", Name: "Uni Test",
		Account: []portal.Step{
			{Kind: portal.Goto, Name: "Open registration", Value: base + "/register"},
			{Kind: portal.Fill, Name: "Email", Selector: "#email", Value: "{{student.email}}"},
			{Kind: portal.Fill, Name: "Password", Selector: "#pw", Value: "{{portal.password}}"},
			{Kind: portal.Click, Name: "Create", Selector: "#create"},
			{Kind: portal.Expect, Name: "Code page", Selector: "#t", Contains: "Check your email"},
			{Kind: portal.OTP, Name: "Emailed code", Selector: "#code", CodeRe: `code is (\d{6})`},
			{Kind: portal.Click, Name: "Verify", Selector: "#ok"},
			{Kind: portal.Expect, Name: "Account ready", Selector: "#t", Contains: "Account ready"},
		},
		Steps: []portal.Step{
			{Kind: portal.Goto, Name: "Open login", Value: base + "/login"},
			{Kind: portal.Fill, Name: "Email", Selector: "#email", Value: "{{portal.username}}"},
			{Kind: portal.Fill, Name: "Password", Selector: "#pw", Value: "{{portal.password}}"},
			{Kind: portal.Click, Name: "Log in", Selector: "#go"},
			{Kind: portal.Expect, Name: "Form", Selector: "#t", Contains: "Application form"},
			{Kind: portal.Fill, Name: "Name", Selector: "#name", Value: "{{student.full_name}}"},
			{Kind: portal.Fill, Name: "GPA", Selector: "#gpa", Value: "{{profile.gpa}}"},
			{Kind: portal.Upload, Name: "Passport", Selector: "#passport", DocType: "passport"},
			{Kind: portal.Submit, Name: "Submit", Selector: "#send"},
			{Kind: portal.Expect, Name: "Confirmation", Selector: "#t", Contains: "submitted"},
			{Kind: portal.Read, Name: "Number", Selector: "#ref", Into: "confirmation"},
		}}
}

// Stages 3+4: account with an emailed code, submission, then the decision email.
func TestFullCycleThroughPortalAndInbox(t *testing.T) {
	chrome := chromeOrSkip(t)
	ctx := context.Background()
	s, _ := newTestService(t)
	up := &uniPortal{s: s, t: t, accounts: map[string]string{}, codes: map[string]string{}, active: map[string]bool{}}
	srv := httptest.NewServer(up)
	defer srv.Close()
	runner := &portal.Runner{ChromePath: chrome, Headless: true, StepTimeout: 15 * time.Second, OTPTimeout: 20 * time.Second,
		Inbox: &DBInbox{s: s, poll: 100 * time.Millisecond}}
	reg := portal.NewRegistry(false)
	reg.Register("unitest", portal.FlowAdapter{Flow: uniFlow(srv.URL), Runner: runner})
	s.Portals = reg

	c, sid, app := newApplicant(t, s)
	s.DB.Q().Exec(ctx, `UPDATE universities SET portal = 'unitest', website = 'https://www.unitest.it' WHERE id = (SELECT university_id FROM programs WHERE id = ?)`, app.Program.ID)
	d, _ := s.GetApplication(ctx, c, app.ID)
	for _, it := range d.Checklist.Items {
		if _, err := s.UploadDocument(ctx, c, sid, it.Type, it.Type+".pdf", "", doc(`{"type":"`+it.Type+`"}`)); err != nil {
			t.Fatal(err)
		}
	}
	// no AI in this test: a person accepts each document
	s.DB.Q().Exec(ctx, `UPDATE documents SET status = 'accepted' WHERE student_id = ?`, sid)
	s.UpdateProfile(ctx, c, sid, []byte(`{"degree_target":"master","education_level":"bachelor_done","gpa":"3.6/4","documents_language":"en"}`))
	if _, err := s.Approve(ctx, c, app.ID, "127.0.0.1", "test"); err != nil {
		t.Fatalf("approve: %v", err)
	}

	if n := drain(t, s); n != 2 {
		t.Fatalf("expected 2 jobs (account, submission), ran %d", n)
	}
	d, _ = s.GetApplication(ctx, c, app.ID)
	if d.Status != "submitted" || d.PortalRef == nil || *d.PortalRef != "UT-2027-0099" {
		t.Fatalf("status %s note %v ref %v", d.Status, d.StatusNote, d.PortalRef)
	}
	if up.submitted != 1 || up.got["gpa"] != "3.6/4" {
		t.Errorf("portal got %v (%d submissions)", up.got, up.submitted)
	}
	var accStatus, accUser string
	s.DB.Q().QueryRow(ctx, `SELECT status, username FROM portal_accounts WHERE student_id = ?`, sid).Scan(&accStatus, &accUser)
	if accStatus != "active" || !strings.HasSuffix(accUser, "@inbox.test") {
		t.Errorf("portal account %s %s", accStatus, accUser)
	}

	// The university writes back. A newsletter from another university is ignored;
	// the decision email moves the application.
	if err := s.ReceiveEmail(ctx, InboundEmail{To: accUser, From: "news@other-uni.edu", Subject: "Congratulations on your interest", Text: "Open day!"}); err != nil {
		t.Fatal(err)
	}
	if d, _ = s.GetApplication(ctx, c, app.ID); d.Decision != nil {
		t.Fatalf("an email from another university must not set a decision: %v", *d.Decision)
	}
	if err := s.ReceiveEmail(ctx, InboundEmail{To: accUser, From: "Admissions <admissions@unitest.it>", Subject: "Admission decision",
		Text: "Dear Islom, we are pleased to offer you admission to the programme."}); err != nil {
		t.Fatal(err)
	}
	d, _ = s.GetApplication(ctx, c, app.ID)
	if d.Decision == nil || *d.Decision != "admitted" {
		t.Fatalf("decision: %v", d.Decision)
	}
	var kinds []string
	for _, e := range d.Events {
		kinds = append(kinds, e.Kind)
	}
	want := "created documents_complete approved account_created submitting submitted decision_admitted"
	if got := strings.Join(kinds, " "); got != want {
		t.Errorf("timeline:\n got  %s\n want %s", got, want)
	}
	// Mail bodies are stored encrypted.
	var body []byte
	s.DB.Q().QueryRow(ctx, `SELECT body_ct FROM inbox_messages WHERE subject = 'Admission decision'`).Scan(&body)
	if len(body) == 0 || strings.Contains(string(body), "pleased") {
		t.Error("inbox body must be stored encrypted")
	}
}

// Stage 4 guard: never submit after the deadline.
func TestNoSubmissionAfterDeadline(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestService(t)
	appIDs, cl := readyApplication(t, s, `{"degree_target":"master","education_level":"bachelor_done","gpa":"3.4/4","documents_language":"en"}`)
	appID := uuid.MustParse(appIDs)
	var pid string
	s.DB.Q().QueryRow(ctx, `SELECT program_id FROM applications WHERE id = ?`, appID).Scan(&pid)
	s.DB.Q().Exec(ctx, `DELETE FROM program_deadlines WHERE program_id = ?`, pid)
	s.DB.Q().Exec(ctx, `INSERT INTO program_deadlines(program_id, academic_year, round, closes_on, kind, source_url, verified_at) VALUES (?, '2026/27', 'Only round', '2026-01-15', 'official', 'https://example.edu', '2025-10-01')`, pid)
	drain(t, s)
	d, _ := s.GetApplication(ctx, cl.(*auth.Claims), appID)
	if d.Status != "needs_attention" || d.StatusNote == nil || !strings.Contains(*d.StatusNote, "deadline") {
		t.Fatalf("expected parked for deadline, got %s %v", d.Status, d.StatusNote)
	}
}

var _ = io.Discard
