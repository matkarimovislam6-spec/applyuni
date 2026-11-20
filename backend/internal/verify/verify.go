// Package verify decides what happens to a document after the AI has read
// it. The AI extracts facts; the rules here are plain code, so every
// acceptance or rejection is explainable and testable:
//
//   - Check judges one document on its own (right type, legible, still valid).
//   - CrossCheck compares the student's documents with each other (same
//     name and date of birth as the passport).
//
// Outcomes: accepted, rejected (the student must upload a new file, with a
// reason they can act on) or needs_review (a person looks before we decide;
// we never reject a student because the machine is unsure).
package verify

import (
	"fmt"
	"strings"
	"time"
)

const (
	Accepted    = "accepted"
	Rejected    = "rejected"
	NeedsReview = "needs_review"
)

// Extraction is what the AI read from one file. Field keys are the canonical
// ones listed in Spec.Fields; dates are YYYY-MM-DD.
type Extraction struct {
	DetectedType    string            `json:"detected_type"`
	MatchesExpected bool              `json:"matches_expected"`
	Legible         bool              `json:"legible"`
	Confidence      float64           `json:"confidence"` // 0..1, the AI's own certainty
	Language        string            `json:"language"`   // ISO 639-1 of the document text
	HasApostille    bool              `json:"has_apostille"`
	Fields          map[string]string `json:"fields"`
	Issues          []string          `json:"issues"`
	Accept          bool              `json:"accept"`
}

// Spec describes one document type: what to read from it and its own rules.
type Spec struct {
	Type   string
	Fields []string // canonical keys the AI must extract
	Hint   string   // what the document looks like, for the AI
	// MinValidMonths: the document must not expire within this many months
	// (passport: the visa and the first year of study).
	MinValidMonths int
	// MaxAgeMonths: results older than this are no longer accepted
	// (English tests are valid for two years).
	MaxAgeMonths int
	DateField    string // field MaxAgeMonths is measured from
}

var specs = map[string]Spec{
	"passport": {Type: "passport", Fields: []string{"full_name", "date_of_birth", "document_number", "nationality", "expiry_date"},
		Hint: "The photo page of an international (foreign travel) passport with the machine-readable zone. An internal ID card is not accepted.", MinValidMonths: 15},
	"photo": {Type: "photo", Fields: []string{},
		Hint: "A recent passport-style portrait photo, face fully visible, plain light background."},
	"school_diploma": {Type: "school_diploma", Fields: []string{"full_name", "institution", "issue_date"},
		Hint: "A school leaving certificate (attestat) confirming completed secondary education."},
	"school_transcript": {Type: "school_transcript", Fields: []string{"full_name", "institution", "grade_average", "grade_scale"},
		Hint: "The list of school subjects with final grades (the attestat supplement)."},
	"bachelor_diploma": {Type: "bachelor_diploma", Fields: []string{"full_name", "institution", "degree_name", "issue_date"},
		Hint: "A bachelor's degree certificate."},
	"bachelor_transcript": {Type: "bachelor_transcript", Fields: []string{"full_name", "institution", "grade_average", "grade_scale"},
		Hint: "The list of university courses with grades and credits (diploma supplement or transcript of records)."},
	"english_certificate": {Type: "english_certificate", Fields: []string{"full_name", "date_of_birth", "test_name", "overall_score", "test_date"},
		Hint: "An IELTS, TOEFL iBT, Duolingo English Test or Cambridge result.", MaxAgeMonths: 24, DateField: "test_date"},
	"german_certificate": {Type: "german_certificate", Fields: []string{"full_name", "test_name", "overall_score", "test_date"},
		Hint: "A TestDaF, DSH or Goethe-Zertifikat result."},
	"official_translations": {Type: "official_translations", Fields: []string{"full_name", "translator"},
		Hint: "A certified (notarised) translation of a diploma or transcript, with the translator's stamp."},
	"cv":                     {Type: "cv", Fields: []string{"full_name"}, Hint: "A CV or résumé."},
	"motivation_letter":      {Type: "motivation_letter", Fields: []string{"full_name"}, Hint: "A motivation letter or statement of purpose written by the student."},
	"statement_of_purpose":   {Type: "statement_of_purpose", Fields: []string{"full_name"}, Hint: "A statement of purpose written by the student."},
	"essay":                  {Type: "essay", Fields: []string{}, Hint: "A personal essay written by the student."},
	"recommendation_letters": {Type: "recommendation_letters", Fields: []string{"full_name", "recommender"}, Hint: "A signed letter of recommendation from a teacher, professor or employer."},
	"financial_documents":    {Type: "financial_documents", Fields: []string{"full_name", "balance", "currency", "issue_date"}, Hint: "A bank statement or sponsor letter showing available funds.", MaxAgeMonths: 3, DateField: "issue_date"},
}

// SpecFor returns the rules for a document type (a generic one if unknown).
func SpecFor(docType string) Spec {
	if s, ok := specs[docType]; ok {
		return s
	}
	return Spec{Type: docType, Fields: []string{"full_name"}}
}

// Verdict is the decision on one document.
type Verdict struct {
	Status string   `json:"status"`
	Issues []string `json:"issues"` // shown to the student
	Notes  []string `json:"notes"`  // for the reviewer
}

func (v *Verdict) worsen(status, issue string) {
	rank := map[string]int{Accepted: 0, NeedsReview: 1, Rejected: 2}
	if rank[status] > rank[v.Status] {
		v.Status = status
	}
	if issue != "" {
		v.Issues = append(v.Issues, issue)
	}
}

// MinConfidence below which a person reviews the AI's reading.
const MinConfidence = 0.6

// Check judges one document on its own.
func Check(docType string, e Extraction, now time.Time) Verdict {
	spec := SpecFor(docType)
	v := Verdict{Status: Accepted, Issues: []string{}, Notes: []string{}}

	if !e.MatchesExpected {
		what := strings.ReplaceAll(e.DetectedType, "_", " ")
		if what == "" || what == "unknown" {
			what = "a different document"
		}
		v.worsen(Rejected, fmt.Sprintf("This looks like %s, not the %s we asked for.", article(what), strings.ReplaceAll(docType, "_", " ")))
		return v
	}
	if !e.Legible {
		v.worsen(Rejected, "Part of the document can't be read. Please upload a sharper scan or photo with all edges visible.")
		return v
	}

	if spec.MinValidMonths > 0 {
		exp, ok := date(e.Fields["expiry_date"])
		switch {
		case !ok:
			v.worsen(NeedsReview, "")
			v.Notes = append(v.Notes, "expiry date not read")
		case exp.Before(now):
			v.worsen(Rejected, fmt.Sprintf("This document expired on %s.", exp.Format("2 January 2006")))
		case exp.Before(now.AddDate(0, spec.MinValidMonths, 0)):
			v.worsen(Rejected, fmt.Sprintf("Your passport expires on %s. For the visa and your first year of study it must be valid for at least %d more months, so please renew it first.",
				exp.Format("2 January 2006"), spec.MinValidMonths))
		}
	} else if exp, ok := date(e.Fields["expiry_date"]); ok && exp.Before(now) {
		v.worsen(Rejected, fmt.Sprintf("This document expired on %s.", exp.Format("2 January 2006")))
	}

	if spec.MaxAgeMonths > 0 {
		if d, ok := date(e.Fields[spec.DateField]); ok && d.AddDate(0, spec.MaxAgeMonths, 0).Before(now) {
			v.worsen(Rejected, fmt.Sprintf("This is from %s. Universities only accept it within %d months, so you need a newer one.",
				d.Format("January 2006"), spec.MaxAgeMonths))
		}
	}

	var missing []string
	for _, f := range spec.Fields {
		if strings.TrimSpace(e.Fields[f]) == "" {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		v.worsen(NeedsReview, "")
		v.Notes = append(v.Notes, "not read: "+strings.Join(missing, ", "))
	}
	if e.Confidence > 0 && e.Confidence < MinConfidence {
		v.worsen(NeedsReview, "")
		v.Notes = append(v.Notes, fmt.Sprintf("AI confidence %.2f", e.Confidence))
	}
	// The AI saw a problem none of our rules explain: a person decides.
	if !e.Accept && v.Status == Accepted {
		v.worsen(NeedsReview, "")
		v.Notes = append(v.Notes, "AI did not accept: "+strings.Join(e.Issues, "; "))
	}
	return v
}

// Facts are one document's extracted identity, for cross-checking.
type Facts struct {
	DocumentID string
	Type       string
	FullName   string
	BirthDate  string
}

// CrossCheck compares every document with the passport. It returns, per
// document ID, the problems found. Without a readable passport there is
// nothing to compare against yet.
func CrossCheck(docs []Facts) map[string][]string {
	out := map[string][]string{}
	var ref *Facts
	for i := range docs {
		if docs[i].Type == "passport" && docs[i].FullName != "" {
			ref = &docs[i]
			break
		}
	}
	if ref == nil {
		return out
	}
	refDOB, refHasDOB := date(ref.BirthDate)
	for _, d := range docs {
		if d.DocumentID == ref.DocumentID {
			continue
		}
		if d.FullName != "" && !NamesMatch(d.FullName, ref.FullName) {
			out[d.DocumentID] = append(out[d.DocumentID],
				fmt.Sprintf("The name on this document (%s) doesn't match your passport (%s).", d.FullName, ref.FullName))
		}
		if dob, ok := date(d.BirthDate); ok && refHasDOB && !dob.Equal(refDOB) {
			out[d.DocumentID] = append(out[d.DocumentID],
				fmt.Sprintf("The date of birth on this document (%s) is different from your passport (%s).", dob.Format("2 January 2006"), refDOB.Format("2 January 2006")))
		}
	}
	return out
}

func date(s string) (time.Time, bool) {
	for _, layout := range []string{"2006-01-02", "02.01.2006", "2006-01"} {
		if t, err := time.Parse(layout, strings.TrimSpace(s)); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func article(s string) string {
	if s == "" {
		return s
	}
	if strings.ContainsRune("aeiou", rune(s[0])) {
		return "an " + s
	}
	return "a " + s
}
