package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/matkarimovislam6-spec/applyuni/backend/internal/portal"
)

// A one-page test portal: name, GPA, passport upload, submit, confirmation.
type miniPortal struct {
	mu        sync.Mutex
	got       map[string]string
	passport  []byte
	submitted int
}

func (m *miniPortal) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w.Header().Set("Content-Type", "text/html")
	switch r.Method + " " + r.URL.Path {
	case "GET /apply":
		fmt.Fprint(w, `<h1 id="t">Apply</h1><form method="post" action="/apply" enctype="multipart/form-data">
			<input id="name" name="name"><input id="gpa" name="gpa"><input id="passport" name="passport" type="file">
			<button id="send" type="submit">Submit</button></form>`)
	case "POST /apply":
		r.ParseMultipartForm(5 << 20)
		m.got = map[string]string{"name": r.FormValue("name"), "gpa": r.FormValue("gpa")}
		if f, _, err := r.FormFile("passport"); err == nil {
			m.passport, _ = io.ReadAll(f)
		}
		m.submitted++
		http.Redirect(w, r, "/done", http.StatusSeeOther)
	case "GET /done":
		fmt.Fprint(w, `<h1 id="t">Thank you</h1><p>Reference <b id="ref">REF-777</b></p>`)
	default:
		http.NotFound(w, r)
	}
}

func miniFlow(base string) portal.Flow {
	return portal.Flow{Portal: "mini", Name: "Mini portal", Steps: []portal.Step{
		{Kind: portal.Goto, Name: "Open", Value: base + "/apply"},
		{Kind: portal.Fill, Name: "Name", Selector: "#name", Value: "{{student.full_name}}"},
		{Kind: portal.Fill, Name: "GPA", Selector: "#gpa", Value: "{{profile.gpa}}"},
		{Kind: portal.Upload, Name: "Passport", Selector: "#passport", DocType: "passport"},
		{Kind: portal.Submit, Name: "Submit", Selector: "#send"},
		{Kind: portal.Expect, Name: "Confirmation", Selector: "#t", Contains: "Thank you"},
		{Kind: portal.Read, Name: "Reference", Selector: "#ref", Into: "confirmation"},
	}}
}

func chromeOrSkip(t *testing.T) string {
	for _, c := range []string{"google-chrome", "chromium", "chromium-browser"} {
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	t.Skip("Chrome not installed")
	return ""
}

// readyApplication creates a student whose application is approved and queued.
func readyApplication(t *testing.T, s *Service, profile string) (appID string, c any) {
	t.Helper()
	ctx := context.Background()
	sess, err := s.Register(ctx, RegisterInput{Username: fmt.Sprintf("u%d", time.Now().UnixNano()%1e9), Password: "supersecret1"})
	if err != nil {
		t.Fatal(err)
	}
	cl, _ := s.Tokens.Parse(sess.Token)
	me, _ := s.Me(ctx, cl)
	sid := *me.StudentID
	if _, err := s.UpdateProfile(ctx, cl, sid, []byte(profile)); err != nil {
		t.Fatal(err)
	}
	progs, _ := s.ListPrograms(ctx, "IT", "master")
	app, err := s.CreateApplication(ctx, cl, sid, progs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := s.GetApplication(ctx, cl, app.ID)
	for _, it := range d.Checklist.Items {
		if _, err := s.UploadDocument(ctx, cl, sid, it.Type, it.Type+".pdf", "application/pdf", []byte("%PDF-1.4 "+it.Type)); err != nil {
			t.Fatal(err)
		}
	}
	s.DB.Q().Exec(ctx, `UPDATE documents SET status = 'accepted' WHERE student_id = ?`, sid)
	s.UpdateProfile(ctx, cl, sid, []byte(profile)) // refresh status to ready_for_review
	if _, err := s.Approve(ctx, cl, app.ID, "127.0.0.1", "test"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	// point this programme's university at the mini portal
	s.DB.Q().Exec(ctx, `UPDATE universities SET portal = 'mini' WHERE id = (SELECT university_id FROM programs WHERE id = ?)`, progs[0].ID)
	return app.ID.String(), cl
}

func TestWorkerSubmitsThroughBrowser(t *testing.T) {
	chrome := chromeOrSkip(t)
	ctx := context.Background()
	s, _ := newTestService(t)
	mp := &miniPortal{}
	srv := httptest.NewServer(mp)
	defer srv.Close()
	reg := portal.NewRegistry(false)
	reg.Register("mini", portal.FlowAdapter{Flow: miniFlow(srv.URL), Runner: &portal.Runner{ChromePath: chrome, Headless: true, StepTimeout: 15 * time.Second}})
	s.Portals = reg

	appID, _ := readyApplication(t, s, `{"degree_target":"master","education_level":"bachelor_done","gpa":"3.4/4","documents_language":"en"}`)
	if worked, err := s.RunOne(ctx); !worked || err != nil {
		t.Fatalf("worker: %v %v", worked, err)
	}

	var status, ref string
	s.DB.Q().QueryRow(ctx, `SELECT status, coalesce(portal_ref, '') FROM applications WHERE id = ?`, appID).Scan(&status, &ref)
	if status != "submitted" || ref != "REF-777" || mp.submitted != 1 {
		t.Fatalf("status %s ref %q portal submissions %d", status, ref, mp.submitted)
	}
	if !strings.HasPrefix(mp.got["name"], "u") || mp.got["gpa"] != "3.4/4" || !bytes.Equal(mp.passport, []byte("%PDF-1.4 passport")) {
		t.Errorf("portal received %v, passport %q", mp.got, mp.passport)
	}
	// receipt stored as an encrypted document
	var receipts int
	s.DB.Q().QueryRow(ctx, `SELECT count(*) FROM documents WHERE doc_type = 'submission_receipt' AND content_type = 'image/png'`).Scan(&receipts)
	if receipts != 1 {
		t.Errorf("expected 1 receipt, got %d", receipts)
	}
	// portal login stored, password encrypted (not readable in the row)
	var user string
	var secret []byte
	if err := s.DB.Q().QueryRow(ctx, `SELECT username, secret_ct FROM portal_accounts WHERE portal = 'mini'`).Scan(&user, &secret); err != nil {
		t.Fatalf("portal account not stored: %v", err)
	}
	if !strings.HasSuffix(user, "@inbox.test") || len(secret) < 20 {
		t.Errorf("portal account: user %q, secret %d bytes", user, len(secret))
	}
}

func TestWorkerParksMissingDataImmediately(t *testing.T) {
	chrome := chromeOrSkip(t)
	ctx := context.Background()
	s, _ := newTestService(t)
	mp := &miniPortal{}
	srv := httptest.NewServer(mp)
	defer srv.Close()
	reg := portal.NewRegistry(false)
	reg.Register("mini", portal.FlowAdapter{Flow: miniFlow(srv.URL), Runner: &portal.Runner{ChromePath: chrome, Headless: true}})
	s.Portals = reg

	// no GPA in the profile: the flow needs {{profile.gpa}}
	appID, _ := readyApplication(t, s, `{"degree_target":"master","education_level":"bachelor_done","documents_language":"en"}`)
	s.RunOne(ctx)

	var status, note, jobStatus string
	var attempts int
	s.DB.Q().QueryRow(ctx, `SELECT status, coalesce(status_note, '') FROM applications WHERE id = ?`, appID).Scan(&status, &note)
	s.DB.Q().QueryRow(ctx, `SELECT status, attempts FROM jobs WHERE kind = 'submit_application'`).Scan(&jobStatus, &attempts)
	if status != "needs_attention" || !strings.Contains(note, "profile.gpa") || jobStatus != "failed" || attempts != 1 {
		t.Fatalf("expected immediate park with a clear note; got status=%s note=%q job=%s attempts=%d", status, note, jobStatus, attempts)
	}
	if mp.submitted != 0 {
		t.Fatal("nothing may be submitted when data is missing")
	}
}
