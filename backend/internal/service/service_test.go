package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"github.com/matkarimovislam6-spec/applyuni/backend/internal/auth"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/crypto"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/db"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/portal"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/requirements"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/storage"
)

// newTestService builds a full service on a fresh SQLite file, or on
// PostgreSQL when TEST_POSTGRES_URL points at an empty database.
func newTestService(t *testing.T) (*Service, string) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	url := "sqlite:" + filepath.Join(dir, "test.db")
	if pg := os.Getenv("TEST_POSTGRES_URL"); pg != "" {
		url = pg
	}
	d, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := d.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	k := make([]byte, 32)
	rand.Read(k)
	kek, _ := crypto.NewLocalKEK(base64.StdEncoding.EncodeToString(k))
	objects := filepath.Join(dir, "objects")
	store, _ := storage.NewLocal(objects)
	return &Service{
		DB: d, Env: crypto.NewEnvelope(kek), Store: store, Portals: portal.NewRegistry(true),
		Tokens: auth.NewIssuer("test-secret-test-secret-test-secret-1234"), MailDomain: "inbox.test",
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, objects
}

func claimsOf(t *testing.T, s *Service, sess *Session) *auth.Claims {
	c, err := s.Tokens.Parse(sess.Token)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

var pdf = []byte("%PDF-1.4\n1 0 obj<<>>endobj\ntrailer<<>>\n%%EOF\n")

func TestCoreFlow(t *testing.T) {
	ctx := context.Background()
	s, objects := newTestService(t)

	// --- B2C student signs up; their student record is created automatically.
	sess, err := s.Register(ctx, RegisterInput{Username: "Aziz_K", Password: "supersecret1", ConfirmPassword: "supersecret1"})
	if err != nil {
		t.Fatal(err)
	}
	for name, in := range map[string]RegisterInput{
		"taken (other case)": {Username: "aziz_k", Password: "supersecret1"},
	} {
		if _, err := s.Register(ctx, in); !errors.Is(err, ErrConflict) {
			t.Errorf("%s: expected conflict, got %v", name, err)
		}
	}
	for name, in := range map[string]RegisterInput{
		"mismatch":          {Username: "bobur", Password: "supersecret1", ConfirmPassword: "supersecret2"},
		"short password":    {Username: "bobur", Password: "short"},
		"password=username": {Username: "bobur123", Password: "Bobur123"},
		"bad username":      {Username: "a!", Password: "supersecret1"},
		"too short name":    {Username: "ab", Password: "supersecret1"},
	} {
		if _, err := s.Register(ctx, in); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: expected invalid, got %v", name, err)
		}
	}
	if _, err := s.Login(ctx, "AZIZ_K", "supersecret1"); err != nil {
		t.Fatalf("login should ignore username case: %v", err)
	}
	if _, err := s.Login(ctx, "aziz_k", "wrong-password"); !errors.Is(err, ErrBadLogin) {
		t.Fatalf("wrong password should fail, got %v", err)
	}
	if _, err := s.Login(ctx, "nobody", "supersecret1"); !errors.Is(err, ErrBadLogin) {
		t.Fatalf("unknown user should fail the same way, got %v", err)
	}
	c := claimsOf(t, s, sess)
	me, err := s.Me(ctx, c)
	if err != nil || me.StudentID == nil || me.OrgKind != "personal" || me.Username != "aziz_k" {
		t.Fatalf("me: %+v %v", me, err)
	}
	sid := *me.StudentID

	// --- Profile drives the checklist.
	prof, _ := json.Marshal(map[string]any{"school_years": 11, "documents_language": "uz", "wants_scholarship": true, "english_test": "IELTS"})
	if _, err := s.UpdateProfile(ctx, c, sid, prof); err != nil {
		t.Fatal(err)
	}
	progs, err := s.ListPrograms(ctx, "it", "bachelor")
	if err != nil || len(progs) == 0 {
		t.Fatalf("programs: %d %v", len(progs), err)
	}
	app, err := s.CreateApplication(ctx, c, sid, progs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateApplication(ctx, c, sid, progs[0].ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("second application to same programme should conflict, got %v", err)
	}
	d, _ := s.GetApplication(ctx, c, app.ID)
	if d.Status != "collecting_documents" || len(d.Checklist.Items) < 8 {
		t.Fatalf("unexpected start state: %s, %d items", d.Status, len(d.Checklist.Items))
	}
	if _, err := s.Approve(ctx, c, app.ID, "127.0.0.1", "test"); !errors.Is(err, ErrNotReady) {
		t.Fatalf("approval before documents should fail, got %v", err)
	}

	// --- Upload every required document; stored objects must be ciphertext.
	var firstDoc uuid.UUID
	for _, it := range d.Checklist.Items {
		doc, err := s.UploadDocument(ctx, c, sid, it.Type, it.Type+".pdf", "application/pdf", pdf)
		if err != nil {
			t.Fatalf("upload %s: %v", it.Type, err)
		}
		if firstDoc == uuid.Nil {
			firstDoc = doc.ID
		}
	}
	filepath.Walk(objects, func(p string, info os.FileInfo, _ error) error {
		if info != nil && !info.IsDir() {
			b, _ := os.ReadFile(p)
			if bytes.Contains(b, []byte("%PDF")) {
				t.Errorf("plaintext found in object store: %s", p)
			}
		}
		return nil
	})
	_, got, err := s.DownloadDocument(ctx, c, firstDoc)
	if err != nil || !bytes.Equal(got, pdf) {
		t.Fatalf("download round trip failed: %v", err)
	}

	// Students cannot accept their own documents.
	if _, err := s.ReviewDocument(ctx, c, firstDoc, true, ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("student self-review should be forbidden, got %v", err)
	}

	// Stand-in for the AI: accept all documents, then let the app refresh.
	if _, err := s.DB.Q().Exec(ctx, `UPDATE documents SET status = 'accepted'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateProfile(ctx, c, sid, prof); err != nil {
		t.Fatal(err)
	}
	d, _ = s.GetApplication(ctx, c, app.ID)
	if d.Status != "ready_for_review" || !d.Checklist.Complete {
		t.Fatalf("expected ready_for_review, got %s", d.Status)
	}

	// --- Approve once; a second approval is refused.
	d, err = s.Approve(ctx, c, app.ID, "127.0.0.1", "test")
	if err != nil || d.Status != "approved" || d.Approval == nil {
		t.Fatalf("approve: %v %+v", err, d)
	}
	if _, err := s.Approve(ctx, c, app.ID, "127.0.0.1", "test"); !errors.Is(err, ErrNotReady) {
		t.Fatalf("double approval should fail, got %v", err)
	}

	// --- Worker submits (dry-run portal).
	if worked, err := s.RunOne(ctx); !worked || err != nil {
		t.Fatalf("worker: worked=%v err=%v", worked, err)
	}
	d, _ = s.GetApplication(ctx, c, app.ID)
	if d.Status != "submitted" || d.PortalRef == nil {
		t.Fatalf("expected submitted, got %s", d.Status)
	}

	// --- B2B agency: seats, staff review, and isolation from the B2C tenant.
	asess, err := s.Register(ctx, RegisterInput{Username: "dilnoza", Email: "owner@agency.uz", Password: "supersecret1", FullName: "Dilnoza", AccountType: "agency", OrgName: "Tashkent Study"})
	if err != nil {
		t.Fatal(err)
	}
	ac := claimsOf(t, s, asess)
	var agencyStudent *Student
	for i := 0; i < 5; i++ {
		st, err := s.CreateStudent(ctx, ac, "Student", "")
		if err != nil {
			t.Fatalf("create student %d: %v", i, err)
		}
		agencyStudent = st
	}
	if _, err := s.CreateStudent(ctx, ac, "One too many", ""); !errors.Is(err, ErrSeatLimit) {
		t.Fatalf("expected seat limit, got %v", err)
	}
	if _, err := s.CreateStudent(ctx, c, "x", ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("B2C student must not create students, got %v", err)
	}
	if list, _ := s.ListStudents(ctx, ac); len(list) != 5 {
		t.Fatalf("agency should see exactly its 5 students, saw %d", len(list))
	}
	if list, _ := s.ListStudents(ctx, c); len(list) != 1 {
		t.Fatalf("B2C should see only itself, saw %d", len(list))
	}

	for name, err := range map[string]error{
		"agency reads B2C student":  second(s.GetStudent(ctx, ac, sid)),
		"agency reads B2C app":      second(s.GetApplication(ctx, ac, app.ID)),
		"agency downloads B2C doc":  third(s.DownloadDocument(ctx, ac, firstDoc)),
		"agency reviews B2C doc":    second(s.ReviewDocument(ctx, ac, firstDoc, true, "")),
		"agency deletes B2C doc":    s.DeleteDocument(ctx, ac, firstDoc),
		"B2C reads agency student":  second(s.GetStudent(ctx, c, agencyStudent.ID)),
		"B2C lists agency docs":     second(s.ListDocuments(ctx, c, agencyStudent.ID)),
		"B2C uploads to agency":     second(s.UploadDocument(ctx, c, agencyStudent.ID, "passport", "p.pdf", "application/pdf", pdf)),
		"B2C applies for agency st": second(s.CreateApplication(ctx, c, agencyStudent.ID, progs[0].ID)),
	} {
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: expected not found, got %v", name, err)
		}
	}

	// Staff manual review works within their own tenant.
	adoc, err := s.UploadDocument(ctx, ac, agencyStudent.ID, "passport", "p.pdf", "application/pdf", pdf)
	if err != nil {
		t.Fatal(err)
	}
	if rd, err := s.ReviewDocument(ctx, ac, adoc.ID, false, "Photo page is cropped"); err != nil || rd.Status != "rejected" {
		t.Fatalf("review: %v", err)
	}

	// --- Deleting the B2C student removes rows and ciphertext.
	if err := s.DeleteStudent(ctx, c, sid); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetApplication(ctx, c, app.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("application should be gone, got %v", err)
	}
	left := 0
	filepath.Walk(filepath.Join(objects, c.OrgID.String()), func(_ string, info os.FileInfo, _ error) error {
		if info != nil && !info.IsDir() {
			left++
		}
		return nil
	})
	if left != 0 {
		t.Fatalf("%d encrypted objects left after delete", left)
	}

	// --- Public website pieces.
	pp, err := s.PublicChecklist(ctx, progs[0].ID, requirements.Profile{SchoolYears: 12})
	if err != nil || len(pp.Checklist.Items) == 0 {
		t.Fatalf("public checklist: %v", err)
	}
	if err := s.JoinWaitlist(ctx, WaitlistInput{Email: "A@b.co"}); err != nil {
		t.Fatal(err)
	}
	if err := s.JoinWaitlist(ctx, WaitlistInput{Email: "a@B.co"}); err != nil {
		t.Fatalf("repeat waitlist signup should be a no-op, got %v", err)
	}
}

func second[T any](_ T, err error) error        { return err }
func third[A, B any](_ A, _ B, err error) error { return err }
