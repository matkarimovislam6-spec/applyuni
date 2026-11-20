package service

import (
	"context"
	"testing"
)

// Documents uploaded while AI checks were off get checked once AI is on.
func TestUncheckedDocumentsAreQueuedWhenAITurnsOn(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestService(t)
	s.AI = nil
	c, sid, _ := newApplicant(t, s)
	d, err := s.UploadDocument(ctx, c, sid, "passport", "passport.pdf", "text/plain",
		doc(`{"full_name":"Nilufar Karimova","date_of_birth":"2001-03-05","document_number":"FA1234567","nationality":"Uzbekistan","expiry_date":"2031-01-01"}`))
	if err != nil {
		t.Fatal(err)
	}
	drain(t, s)
	if got, _ := getDocFor(t, s, c, d.ID); got.Status != "needs_review" {
		t.Fatalf("without AI: status %s", got.Status)
	}

	s.AI = fakeAI{}
	n, err := s.QueueUncheckedDocuments(ctx)
	if err != nil || n != 1 {
		t.Fatalf("queued %d, err %v", n, err)
	}
	if n, _ := s.QueueUncheckedDocuments(ctx); n != 0 {
		t.Fatalf("queued the same document twice")
	}
	drain(t, s)
	if got, _ := getDocFor(t, s, c, d.ID); got.Status != "accepted" {
		t.Fatalf("after AI check: status %s verdict %s ai %s", got.Status, got.Verdict, got.AIResult)
	}
}
