package service

import (
	"context"
	"errors"
	"testing"
)

// ApplyUni staff accept documents for a student who applies alone, and the
// application moves on to approval once everything is accepted.
func TestPlatformAdminReviewsAnyApplicant(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestService(t)
	s.AI = nil // documents wait for a person
	student, sid, app := newApplicant(t, s)

	adminSess, err := s.Register(ctx, RegisterInput{Username: "applyuni_staff", Password: "supersecret1", FullName: "Staff"})
	if err != nil {
		t.Fatal(err)
	}
	admin := claimsOf(t, s, adminSess)
	s.PlatformAdmins = map[string]bool{"applyuni_staff": true}

	// Not an admin: the admin endpoints don't exist for you.
	if _, err := s.AdminReviewQueue(ctx, student); !errors.Is(err, ErrNotFound) {
		t.Fatalf("student saw the admin queue: %v", err)
	}

	detail, err := s.GetApplication(ctx, student, app.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range detail.Checklist.Items {
		if _, err := s.UploadDocument(ctx, student, sid, it.Type, it.Type+".pdf", "", doc(it.Type)); err != nil {
			t.Fatalf("upload %s: %v", it.Type, err)
		}
	}
	drain(t, s)

	queue, err := s.AdminReviewQueue(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != len(detail.Checklist.Items) || len(queue[0].Programs) != 1 {
		t.Fatalf("queue has %d items (want %d), programs %v", len(queue), len(detail.Checklist.Items), queue[0].Programs)
	}
	if _, data, err := s.AdminDocumentFile(ctx, admin, queue[0].ID); err != nil || len(data) == 0 {
		t.Fatalf("open file: %v", err)
	}
	if _, _, err := s.AdminDocumentFile(ctx, student, queue[0].ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("student opened the admin file view: %v", err)
	}

	// Reject one with a reason, then accept everything.
	rej, err := s.AdminReviewDocument(ctx, admin, queue[0].ID, false, "The scan is cut off at the bottom.")
	if err != nil || rej.Status != "rejected" {
		t.Fatalf("reject: %v %v", rej, err)
	}
	for _, it := range queue {
		if _, err := s.AdminReviewDocument(ctx, admin, it.ID, true, ""); err != nil {
			t.Fatalf("accept %s: %v", it.DocType, err)
		}
	}
	detail, _ = s.GetApplication(ctx, student, app.ID)
	if detail.Status != "ready_for_review" {
		t.Fatalf("application status %s after all documents accepted", detail.Status)
	}
	if q, _ := s.AdminReviewQueue(ctx, admin); len(q) != 0 {
		t.Fatalf("queue not empty: %d", len(q))
	}
}
