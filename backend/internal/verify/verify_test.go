package verify

import (
	"strings"
	"testing"
	"time"
)

func TestNamesMatch(t *testing.T) {
	same := [][2]string{
		{"Islom Matkarimov", "ISLOM MATKARIMOV"},
		{"Ислом Маткаримов", "Islom Matkarimov"},
		{"Islam Matkarimov", "Islom Matkarimov"},                // Russian a / Uzbek o
		{"Matkarimov Islom", "Islom Matkarimov"},                // surname first
		{"Islom Bahodir o'g'li Matkarimov", "Islom Matkarimov"}, // patronymic
		{"Ислом Баходирович Маткаримов", "Islom Matkarimov"},
		{"Muhammad Xolmatov", "Mukhammad Kholmatov"},
		{"Nilufar G'ulomova", "Nilufar Gulomova"},
		{"Oʻtkir Qodirov", "Utkir Kadirov"},
		{"Yelena Kim", "Elena Kim"},
		{"Dilnoza Yusupova", "Dilnoza Iusupova"},
	}
	for _, p := range same {
		if !NamesMatch(p[0], p[1]) {
			t.Errorf("should match: %q vs %q", p[0], p[1])
		}
	}
	diff := [][2]string{
		{"Islom Matkarimov", "Aziz Matkarimov"}, // sibling
		{"Nilufar Karimova", "Nilufar Rahimova"},
		{"Islom Matkarimov", "Islom"}, // surname missing: not enough
		{"Dilshod Aliyev", "Dilnoza Aliyeva"},
		{"", "Islom Matkarimov"},
	}
	for _, p := range diff {
		if NamesMatch(p[0], p[1]) {
			t.Errorf("should NOT match: %q vs %q", p[0], p[1])
		}
	}
}

var now = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

func ok(fields map[string]string) Extraction {
	return Extraction{MatchesExpected: true, Legible: true, Confidence: 0.95, Accept: true, Fields: fields}
}

func TestCheck(t *testing.T) {
	passport := map[string]string{"full_name": "Islom Matkarimov", "date_of_birth": "2004-03-01", "document_number": "FA1234567", "nationality": "UZB", "expiry_date": "2031-05-01"}
	cases := []struct {
		name, docType string
		e             Extraction
		want, issue   string
	}{
		{"good passport", "passport", ok(passport), Accepted, ""},
		{"wrong document", "passport", Extraction{DetectedType: "id_card", Legible: true}, Rejected, "an id card"},
		{"blurry", "passport", Extraction{MatchesExpected: true}, Rejected, "can't be read"},
		{"passport expires soon", "passport", ok(with(passport, "expiry_date", "2027-06-01")), Rejected, "renew"},
		{"passport expired", "passport", ok(with(passport, "expiry_date", "2025-01-01")), Rejected, "expired"},
		{"expiry unread", "passport", ok(with(passport, "expiry_date", "")), NeedsReview, ""},
		{"old IELTS", "english_certificate", ok(map[string]string{"full_name": "I M", "date_of_birth": "2004-03-01", "test_name": "IELTS", "overall_score": "7.0", "test_date": "2024-05-10"}), Rejected, "within 24 months"},
		{"fresh IELTS", "english_certificate", ok(map[string]string{"full_name": "I M", "date_of_birth": "2004-03-01", "test_name": "IELTS", "overall_score": "7.0", "test_date": "2026-05-10"}), Accepted, ""},
		{"unsure AI", "school_diploma", Extraction{MatchesExpected: true, Legible: true, Confidence: 0.4, Accept: true, Fields: map[string]string{"full_name": "a", "institution": "b", "issue_date": "2022-06-01"}}, NeedsReview, ""},
		{"AI objects, no rule", "school_diploma", Extraction{MatchesExpected: true, Legible: true, Confidence: 0.9, Accept: false, Issues: []string{"stamp missing"}, Fields: map[string]string{"full_name": "a", "institution": "b", "issue_date": "2022-06-01"}}, NeedsReview, ""},
	}
	for _, c := range cases {
		v := Check(c.docType, c.e, now)
		if v.Status != c.want {
			t.Errorf("%s: status %s, want %s (%v %v)", c.name, v.Status, c.want, v.Issues, v.Notes)
		}
		if c.issue != "" && !strings.Contains(strings.Join(v.Issues, " "), c.issue) {
			t.Errorf("%s: issues %v should mention %q", c.name, v.Issues, c.issue)
		}
	}
}

func with(m map[string]string, k, v string) map[string]string {
	out := map[string]string{}
	for a, b := range m {
		out[a] = b
	}
	out[k] = v
	return out
}

func TestCrossCheck(t *testing.T) {
	got := CrossCheck([]Facts{
		{DocumentID: "p", Type: "passport", FullName: "MATKARIMOV ISLOM", BirthDate: "2004-03-01"},
		{DocumentID: "d", Type: "school_diploma", FullName: "Маткаримов Ислом Баходирович"},
		{DocumentID: "i", Type: "english_certificate", FullName: "Islom Matkarimov", BirthDate: "2004-01-03"},
		{DocumentID: "x", Type: "bachelor_transcript", FullName: "Aziz Matkarimov"},
	})
	if len(got["d"]) != 0 {
		t.Errorf("Cyrillic diploma should match: %v", got["d"])
	}
	if len(got["i"]) != 1 || !strings.Contains(got["i"][0], "date of birth") {
		t.Errorf("IELTS with swapped day/month should be flagged: %v", got["i"])
	}
	if len(got["x"]) != 1 || !strings.Contains(got["x"][0], "doesn't match your passport") {
		t.Errorf("other person's transcript should be flagged: %v", got["x"])
	}
	if len(CrossCheck([]Facts{{DocumentID: "d", Type: "school_diploma", FullName: "A B"}})) != 0 {
		t.Error("no passport: nothing to compare")
	}
}
