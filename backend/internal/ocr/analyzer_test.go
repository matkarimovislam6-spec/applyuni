package ocr

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/matkarimovislam6-spec/applyuni/backend/internal/verify"
)

type textEngine struct {
	text string
	err  error
}

func (e textEngine) Name() string { return "fake" }
func (e textEngine) Text(context.Context, string, []byte) (string, error) {
	return e.text, e.err
}

func analyze(t *testing.T, docType, text string, known verify.Known) (verify.Verdict, map[string]any) {
	t.Helper()
	a := &Analyzer{Engine: textEngine{text: text}, Now: func() time.Time { return now }}
	e, raw, err := a.AnalyzeKnown(context.Background(), docType, "image/png", []byte("x"), known)
	if err != nil {
		t.Fatal(err)
	}
	var stored map[string]any
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	return verify.Check(docType, *e, now), stored
}

// A valid passport with years left (expires 2032) is accepted on OCR alone.
var validPassport = "PASSPORT\nP<UTOERIKSSON<<ANNA<MARIA<<<<<<<<<<<<<<<<<<<\n" + td3Line2("L898902C3", "UTO", "740812", "F", "320415")

// td3Line2 builds a passport's second MRZ line with correct check digits.
func td3Line2(number, nationality, birth, sex, expiry string) string {
	d := func(s string) string { return string(rune('0' + checkDigit(s))) }
	opt := "<<<<<<<<<<<<<<"
	head := number + d(number) + nationality + birth + d(birth) + sex + expiry + d(expiry) + opt + "0"
	composite := number + d(number) + birth + d(birth) + expiry + d(expiry) + opt + "0"
	return head + d(composite)
}

func TestPassportAcceptedFromMRZ(t *testing.T) {
	v, stored := analyze(t, "passport", validPassport, verify.Known{FullName: "Anna Maria Eriksson", BirthDate: "1974-08-12"})
	if v.Status != verify.Accepted {
		t.Fatalf("%+v %v", v, stored["ocr_findings"])
	}
	if stored["reader"] != "ocr/fake" {
		t.Fatal(stored["reader"])
	}
}

func TestExpiredPassportIsRejectedWithReason(t *testing.T) {
	v, _ := analyze(t, "passport", specimenTD3, verify.Known{}) // expired 2012
	if v.Status != verify.Rejected || !strings.Contains(v.Issues[0], "expired") {
		t.Fatalf("%+v", v)
	}
}

func TestPassportNameDiffersFromProfileGoesToAPerson(t *testing.T) {
	v, _ := analyze(t, "passport", validPassport, verify.Known{FullName: "Islom Matkarimov"})
	if v.Status != verify.NeedsReview {
		t.Fatalf("%+v", v)
	}
}

func TestIDCardUploadedAsPassportIsRejected(t *testing.T) {
	text := "I<UTOD231458907<<<<<<<<<<<<<<<\n7408122F1204159UTO<<<<<<<<<<<6\nERIKSSON<<ANNA<MARIA<<<<<<<<<<\n"
	v, _ := analyze(t, "passport", text, verify.Known{})
	if v.Status != verify.Rejected || !strings.Contains(v.Issues[0], "id card") {
		t.Fatalf("%+v", v)
	}
}

func TestUnreadablePassportGoesToAPersonNotRejected(t *testing.T) {
	v, _ := analyze(t, "passport", "PASSPORT blurry text here and more words", verify.Known{})
	if v.Status != verify.NeedsReview {
		t.Fatalf("%+v", v)
	}
}

const ielts = `IELTS Test Report Form ACADEMIC
Centre Number UZ123 Date 14/MAR/2026 Candidate Number 000123
Family Name MATKARIMOV First Name ISLOM
Date of Birth 01/03/2004 Sex M
Listening 7.5 Reading 7.0 Writing 6.5 Speaking 7.0 Overall Band Score 7.0 CEFR Level C1`

func TestIELTSAccepted(t *testing.T) {
	v, stored := analyze(t, "english_certificate", ielts, verify.Known{FullName: "Islom Matkarimov", BirthDate: "2004-03-01"})
	if v.Status != verify.Accepted {
		t.Fatalf("%+v %v", v, stored["ocr_findings"])
	}
	fields := stored["fields"].(map[string]any)
	if fields["overall_score"] != "7.0" || fields["test_date"] != "2026-03-14" {
		t.Fatalf("%v", fields)
	}
}

func TestIELTSOfSomeoneElseGoesToAPerson(t *testing.T) {
	v, _ := analyze(t, "english_certificate", ielts, verify.Known{FullName: "Nilufar Karimova", BirthDate: "2001-05-05"})
	if v.Status != verify.NeedsReview {
		t.Fatalf("%+v", v)
	}
}

func TestOldIELTSIsRejected(t *testing.T) {
	old := strings.Replace(ielts, "14/MAR/2026", "14/MAR/2023", 1)
	v, _ := analyze(t, "english_certificate", old, verify.Known{FullName: "Islom Matkarimov", BirthDate: "2004-03-01"})
	if v.Status != verify.Rejected {
		t.Fatalf("%+v", v)
	}
}

func TestDiplomaGoesToAPersonWithNotes(t *testing.T) {
	text := "O'ZBEKISTON RESPUBLIKASI ATTESTAT ushbu attestat MATKARIMOV ISLOM BAHODIR O'G'LI ga berildi 2022"
	v, stored := analyze(t, "school_diploma", text, verify.Known{FullName: "Islom Matkarimov"})
	if v.Status != verify.NeedsReview {
		t.Fatalf("%+v", v)
	}
	notes := strings.Join(toStrings(stored["ocr_findings"]), " | ")
	if !strings.Contains(notes, "name is on the document") || !strings.Contains(notes, "ATTESTAT") {
		t.Fatal(notes)
	}
}

func TestMotivationLetter(t *testing.T) {
	body := strings.Repeat("I am applying to this university program because I love engineering and design. ", 15)
	letter := "Dear Admission Committee,\n" + body + "\nSincerely, Islom Matkarimov"
	if v, _ := analyze(t, "motivation_letter", letter, verify.Known{FullName: "Islom Matkarimov"}); v.Status != verify.Accepted {
		t.Fatalf("letter: %+v", v)
	}
	if v, _ := analyze(t, "motivation_letter", "Dear committee, I want to study. Islom Matkarimov", verify.Known{FullName: "Islom Matkarimov"}); v.Status != verify.NeedsReview {
		t.Fatalf("short letter: %+v", v)
	}
}

func TestEngineFailureGoesToAPerson(t *testing.T) {
	a := &Analyzer{Engine: textEngine{err: errors.New("boom")}, Now: func() time.Time { return now }}
	e, _, err := a.AnalyzeKnown(context.Background(), "passport", "image/png", nil, verify.Known{})
	if err != nil {
		t.Fatal("an OCR failure must not fail the job")
	}
	if v := verify.Check("passport", *e, now); v.Status != verify.NeedsReview {
		t.Fatalf("%+v", v)
	}
}

type countingAI struct{ calls int }

func (c *countingAI) AnalyzeDocument(context.Context, string, string, []byte) (*verify.Extraction, json.RawMessage, error) {
	c.calls++
	return &verify.Extraction{MatchesExpected: true, Legible: true, Accept: true, Confidence: 0.9, Fields: map[string]string{"full_name": "X"}}, []byte(`{}`), nil
}

// The AI is only paid for when OCR can't accept the document by itself.
func TestChainCallsAIOnlyWhenNeeded(t *testing.T) {
	ai := &countingAI{}
	c := &Chain{OCR: &Analyzer{Engine: textEngine{text: validPassport}, Now: func() time.Time { return now }}, Then: ai}
	if _, _, err := c.AnalyzeKnown(context.Background(), "passport", "image/png", nil, verify.Known{}); err != nil || ai.calls != 0 {
		t.Fatalf("passport: AI called %d times, err %v", ai.calls, err)
	}
	c.OCR.Engine = textEngine{text: "ATTESTAT some school certificate text here"}
	if _, _, _ = c.AnalyzeKnown(context.Background(), "school_diploma", "image/png", nil, verify.Known{}); ai.calls != 1 {
		t.Fatalf("diploma: AI called %d times", ai.calls)
	}
}

func toStrings(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}
