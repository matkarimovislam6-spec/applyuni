package service

import (
	"context"
	"strings"
	"testing"

	"github.com/matkarimovislam6-spec/applyuni/backend/internal/ocr"
)

// textByType returns OCR text depending on which file is being read.
type textByType map[string]string

func (t textByType) Name() string { return "fake" }
func (t textByType) Text(_ context.Context, _ string, data []byte) (string, error) {
	return t[string(data[:strings.IndexByte(string(data), '\n')])], nil
}

// The OCR reader runs in the worker: passports are accepted from their MRZ,
// other documents go to a person with notes, and the student's passport
// name is looked for on later documents.
func TestOCRReaderInWorker(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestService(t)
	s.AI = &ocr.Analyzer{Engine: textByType{
		"%PDF-1.4 passport": "P<UZBMATKARIMOV<<ISLOM<<<<<<<<<<<<<<<<<<<<<<<\nFA12345673UZB0403014M3401011<<<<<<<<<<<<<<02",
		"%PDF-1.4 diploma":  "ATTESTAT ushbu attestat MATKARIMOV ISLOM BAHODIR O'G'LI ga berildi Toshkent 2022",
	}}
	c, sid, _ := newApplicant(t, s)

	pass, err := s.UploadDocument(ctx, c, sid, "passport", "passport.pdf", "", []byte("%PDF-1.4 passport\n%%EOF\n"))
	if err != nil {
		t.Fatal(err)
	}
	drain(t, s)
	pass, _ = getDocFor(t, s, c, pass.ID)
	if pass.Status != "accepted" {
		t.Fatalf("passport %s %s %s", pass.Status, pass.Verdict, pass.AIResult)
	}

	dip, err := s.UploadDocument(ctx, c, sid, "school_diploma", "attestat.pdf", "", []byte("%PDF-1.4 diploma\n%%EOF\n"))
	if err != nil {
		t.Fatal(err)
	}
	drain(t, s)
	dip, _ = getDocFor(t, s, c, dip.ID)
	if dip.Status != "needs_review" || !strings.Contains(string(dip.AIResult), "name is on the document") {
		t.Fatalf("diploma %s %s", dip.Status, dip.AIResult)
	}
}
