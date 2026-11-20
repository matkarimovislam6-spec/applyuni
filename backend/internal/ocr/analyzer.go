package ocr

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/matkarimovislam6-spec/applyuni/backend/internal/verify"
)

// Analyzer reads documents with OCR and plain rules. It implements the
// service's document reader, so verify.Check makes the final decision.
type Analyzer struct {
	Engine Engine
	Now    func() time.Time // for tests; default time.Now
}

// result is what we store as the document's ai_result: the extraction plus
// what OCR found, for the reviewer.
type result struct {
	verify.Extraction
	Reader   string   `json:"reader"`
	Findings []string `json:"ocr_findings"`
}

func (a *Analyzer) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

// AnalyzeDocument reads a document without knowing anything about the student.
func (a *Analyzer) AnalyzeDocument(ctx context.Context, expectedType, contentType string, data []byte) (*verify.Extraction, json.RawMessage, error) {
	return a.AnalyzeKnown(ctx, expectedType, contentType, data, verify.Known{})
}

// AnalyzeKnown reads a document, looking for what we know about the student.
// Problems reading the file never fail the job: the document goes to a person.
func (a *Analyzer) AnalyzeKnown(ctx context.Context, expectedType, contentType string, data []byte, known verify.Known) (*verify.Extraction, json.RawMessage, error) {
	r := &result{Reader: "ocr/" + a.Engine.Name(), Extraction: verify.Extraction{
		MatchesExpected: true, Legible: true, Fields: map[string]string{}, Issues: []string{},
	}}
	text, err := a.Engine.Text(ctx, contentType, data)
	if err != nil {
		r.hold("OCR could not read the file: %v", err)
		return r.out()
	}
	r.Language = language(text)
	words := len(strings.Fields(text))
	if words < 5 && expectedType != "photo" && expectedType != "passport" { // a passport's MRZ is two "words"
		r.hold("OCR found almost no text (%d words); it may be a photo or a very poor scan", words)
		return r.out()
	}

	switch expectedType {
	case "passport":
		a.passport(r, text, known)
	case "english_certificate":
		a.englishTest(r, text, known)
	case "cv", "motivation_letter", "statement_of_purpose":
		a.writtenByStudent(r, expectedType, text, words, known)
	case "photo":
		r.hold("photos are always checked by a person")
	default:
		a.other(r, expectedType, text, known)
	}
	return r.out()
}

// hold sends the document to a person, with a note on why.
func (r *result) hold(format string, args ...any) {
	note := fmt.Sprintf(format, args...)
	r.Accept = false
	r.Issues = append(r.Issues, note)
	r.Findings = append(r.Findings, note)
}

func (r *result) found(format string, args ...any) {
	r.Findings = append(r.Findings, fmt.Sprintf(format, args...))
}

func (r *result) out() (*verify.Extraction, json.RawMessage, error) {
	if r.Findings == nil {
		r.Findings = []string{}
	}
	raw, err := json.Marshal(r)
	return &r.Extraction, raw, err
}

func (a *Analyzer) passport(r *result, text string, known verify.Known) {
	m, err := FindMRZ(text, a.now())
	if err != nil {
		r.hold("passport: the machine-readable zone (the two lines with <<< at the bottom) could not be read: %v", err)
		return
	}
	if m.Format != "TD3" || m.DocumentCode != "P" {
		// Verified check digits: this really is an ID card, not a passport.
		r.MatchesExpected = false
		r.DetectedType = "id_card"
		r.found("machine-readable zone of an ID card (%s, code %s)", m.Format, m.DocumentCode)
		return
	}
	r.DetectedType = "passport"
	r.Confidence = 0.99
	r.Accept = true
	r.Fields = map[string]string{
		"full_name": m.FullName(), "date_of_birth": m.BirthDate, "document_number": m.DocumentNumber,
		"nationality": m.Nationality, "expiry_date": m.ExpiryDate, "sex": m.Sex,
	}
	r.found("machine-readable zone verified: %s, born %s, passport %s, expires %s", m.FullName(), m.BirthDate, m.DocumentNumber, m.ExpiryDate)
	if known.FullName != "" && !verify.NamesMatch(known.FullName, m.FullName()) {
		r.hold("the passport name %q differs from the name in the student's profile %q", m.FullName(), known.FullName)
	}
	if known.BirthDate != "" && known.BirthDate != m.BirthDate {
		r.hold("the passport birth date %s differs from the student's profile (%s)", m.BirthDate, known.BirthDate)
	}
}

var (
	ieltsScore    = regexp.MustCompile(`(?is)overall\s*band\s*score\D{0,40}?(\d(?:[.,]\d)?)\b`)
	toeflScore    = regexp.MustCompile(`(?is)total\s*score\D{0,40}?(\d{2,3})\b`)
	duolingoScore = regexp.MustCompile(`(?is)(?:overall\s*score|overall)\D{0,20}?(\d{2,3})\b`)
	cambridgeScr  = regexp.MustCompile(`(?is)cambridge\s*english\s*scale\D{0,40}?(\d{3})\b`)
)

func (a *Analyzer) englishTest(r *result, text string, known verify.Known) {
	low := strings.ToLower(text)
	var test, score string
	switch {
	case strings.Contains(low, "ielts"):
		test = "IELTS"
		if m := ieltsScore.FindStringSubmatch(text); m != nil {
			score = strings.ReplaceAll(m[1], ",", ".")
			if f, err := strconv.ParseFloat(score, 64); err != nil || f > 9 {
				score = ""
			}
		}
	case strings.Contains(low, "toefl"):
		test = "TOEFL iBT"
		if m := toeflScore.FindStringSubmatch(text); m != nil {
			if n, _ := strconv.Atoi(m[1]); n <= 120 {
				score = m[1]
			}
		}
	case strings.Contains(low, "duolingo"):
		test = "Duolingo English Test"
		if m := duolingoScore.FindStringSubmatch(text); m != nil {
			if n, _ := strconv.Atoi(m[1]); n >= 10 && n <= 160 {
				score = m[1]
			}
		}
	case strings.Contains(low, "cambridge"):
		test = "Cambridge English"
		if m := cambridgeScr.FindStringSubmatch(text); m != nil {
			score = m[1]
		}
	default:
		r.hold("no IELTS, TOEFL, Duolingo or Cambridge wording found")
		return
	}
	r.DetectedType = "english_certificate"
	r.Fields["test_name"] = test
	r.found("test: %s", test)
	if score != "" {
		r.Fields["overall_score"] = score
		r.found("overall score: %s", score)
	}
	dates := findDates(text)
	if known.BirthDate != "" && contains(dates, known.BirthDate) {
		r.Fields["date_of_birth"] = known.BirthDate
		r.found("birth date matches the passport")
	}
	// The test date: the latest date on the report that isn't in the future
	// or the birth date.
	today := a.now().Format("2006-01-02")
	for i := len(dates) - 1; i >= 0; i-- {
		if dates[i] <= today && dates[i] != known.BirthDate {
			r.Fields["test_date"] = dates[i]
			r.found("test date: %s", dates[i])
			break
		}
	}
	a.name(r, text, known)
	r.Confidence = 0.8
	r.Accept = true
	for _, f := range []string{"full_name", "date_of_birth", "overall_score", "test_date"} {
		if r.Fields[f] == "" {
			r.hold("could not confirm %s", strings.ReplaceAll(f, "_", " "))
		}
	}
}

var studentWritten = map[string]*regexp.Regexp{
	"cv":                   regexp.MustCompile(`(?i)\b(experience|education|skills|languages|curriculum|résumé|resume|опыт|образование|навыки|tajriba|ta'lim)\b`),
	"motivation_letter":    regexp.MustCompile(`(?i)\b(motivat\w*|dear|admission|apply|applying|program\w*|university|sincerely)\b`),
	"statement_of_purpose": regexp.MustCompile(`(?i)\b(purpose|research|goal\w*|admission|program\w*|university|sincerely)\b`),
}

// writtenByStudent: CVs and letters are accepted when they carry the
// student's name, read like the document asked for and are of real length.
func (a *Analyzer) writtenByStudent(r *result, docType, text string, words int, known verify.Known) {
	a.name(r, text, known)
	hits := len(studentWritten[docType].FindAllString(text, -1))
	r.found("%d words, %d typical words for a %s", words, hits, strings.ReplaceAll(docType, "_", " "))
	r.DetectedType = docType
	r.Confidence = 0.7
	r.Accept = true
	if r.Fields["full_name"] == "" {
		r.hold("the student's name was not found")
	}
	if words < 120 {
		r.hold("only %d words; too short for a %s", words, strings.ReplaceAll(docType, "_", " "))
	}
	if hits < 2 {
		r.hold("does not read like a %s", strings.ReplaceAll(docType, "_", " "))
	}
}

var typeWords = map[string]*regexp.Regexp{
	"school_diploma":         regexp.MustCompile(`(?i)(attestat|аттестат|certificate of (secondary|general)|o'rta ta'lim|среднем)`),
	"school_transcript":      regexp.MustCompile(`(?i)(ilova|приложение|transcript|baho|оценк|grades?)`),
	"bachelor_diploma":       regexp.MustCompile(`(?i)(diplom|диплом|bachelor|bakalavr|бакалавр)`),
	"bachelor_transcript":    regexp.MustCompile(`(?i)(transcript|ilova|приложение|credits?|kredit|кредит)`),
	"official_translations":  regexp.MustCompile(`(?i)(translat|tarjima|перевод|notar|нотариус)`),
	"recommendation_letters": regexp.MustCompile(`(?i)(recommend|tavsiya|рекоменд)`),
	"financial_documents":    regexp.MustCompile(`(?i)(bank|balance|statement|hisob|выписка|остаток)`),
}

// other documents: OCR can't judge them safely, so a person decides; we
// save them time by noting what was found.
func (a *Analyzer) other(r *result, docType, text string, known verify.Known) {
	a.name(r, text, known)
	if re := typeWords[docType]; re != nil {
		if m := re.FindString(text); m != "" {
			r.found("wording typical of a %s: %q", strings.ReplaceAll(docType, "_", " "), m)
		} else {
			r.found("no wording typical of a %s found", strings.ReplaceAll(docType, "_", " "))
		}
	}
	if known.BirthDate != "" && contains(findDates(text), known.BirthDate) {
		r.Fields["date_of_birth"] = known.BirthDate
		r.found("birth date matches the passport")
	}
	r.hold("%s: checked by a person (OCR only reads passports, language tests, CVs and letters on its own)", strings.ReplaceAll(docType, "_", " "))
}

// name records the student's known name if it is written on the document.
func (a *Analyzer) name(r *result, text string, known verify.Known) {
	if known.FullName == "" {
		r.found("no passport name known yet to look for")
		return
	}
	if verify.NameAppearsIn(known.FullName, text) {
		r.Fields["full_name"] = known.FullName
		r.found("the student's name is on the document")
	} else {
		r.found("the student's name (%s) was not found on the document", known.FullName)
	}
}

func language(text string) string {
	var cyr, lat int
	for _, c := range text {
		switch {
		case unicode.Is(unicode.Cyrillic, c):
			cyr++
		case unicode.Is(unicode.Latin, c):
			lat++
		}
	}
	if cyr > lat {
		return "ru"
	}
	return ""
}

var (
	reISO   = regexp.MustCompile(`\b(\d{4})-(\d{2})-(\d{2})\b`)
	reDMY   = regexp.MustCompile(`\b(\d{1,2})[./](\d{1,2})[./](\d{4})\b`)
	reDMonY = regexp.MustCompile(`(?i)\b(\d{1,2})[\s/.-]*(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*[\s/.-]*(\d{4})\b`)
	months  = map[string]int{"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6, "jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12}
)

// findDates returns every valid date in text as YYYY-MM-DD, sorted, unique.
func findDates(text string) []string {
	set := map[string]bool{}
	add := func(y, m, d int) {
		if t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC); t.Year() == y && int(t.Month()) == m && t.Day() == d && y > 1900 {
			set[t.Format("2006-01-02")] = true
		}
	}
	atoi := func(s string) int { n, _ := strconv.Atoi(s); return n }
	for _, m := range reISO.FindAllStringSubmatch(text, -1) {
		add(atoi(m[1]), atoi(m[2]), atoi(m[3]))
	}
	for _, m := range reDMY.FindAllStringSubmatch(text, -1) {
		add(atoi(m[3]), atoi(m[2]), atoi(m[1]))
	}
	for _, m := range reDMonY.FindAllStringSubmatch(text, -1) {
		add(atoi(m[3]), months[strings.ToLower(m[2])[:3]], atoi(m[1]))
	}
	out := make([]string, 0, len(set))
	for d := range set {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Chain reads with OCR first and asks the AI only when OCR alone can't
// accept the document, so the AI is paid for only on the hard cases.
type Chain struct {
	OCR  *Analyzer
	Then interface {
		AnalyzeDocument(ctx context.Context, expectedType, contentType string, data []byte) (*verify.Extraction, json.RawMessage, error)
	}
}

func (c *Chain) AnalyzeDocument(ctx context.Context, expectedType, contentType string, data []byte) (*verify.Extraction, json.RawMessage, error) {
	return c.AnalyzeKnown(ctx, expectedType, contentType, data, verify.Known{})
}

func (c *Chain) AnalyzeKnown(ctx context.Context, expectedType, contentType string, data []byte, known verify.Known) (*verify.Extraction, json.RawMessage, error) {
	e, raw, err := c.OCR.AnalyzeKnown(ctx, expectedType, contentType, data, known)
	if err == nil && verify.Check(expectedType, *e, c.OCR.now()).Status == verify.Accepted {
		return e, raw, nil
	}
	if c.Then == nil {
		return e, raw, err
	}
	return c.Then.AnalyzeDocument(ctx, expectedType, contentType, data)
}
