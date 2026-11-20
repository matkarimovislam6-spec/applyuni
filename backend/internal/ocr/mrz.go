// Package ocr reads documents without an AI service: text recognition runs on
// our own machine (Apple Vision on macOS, Tesseract on Linux servers), and
// plain rules turn the text into the same Extraction the AI produces.
//
// It is deliberately cautious. It accepts a document only on hard evidence
// (a passport's machine-readable zone with valid check digits, a language
// test report with the student's name, score and date) and otherwise sends
// it to a person with notes on what it found. It never rejects a student
// because it is unsure.
package ocr

import (
	"fmt"
	"strings"
	"time"
)

// MRZ is the machine-readable zone of a passport (TD3: 2 lines of 44) or an
// ID card (TD1: 3 lines of 30), decoded and verified with its check digits.
type MRZ struct {
	Format         string // "TD3" (passport) or "TD1" (ID card)
	DocumentCode   string // "P" passport; "I", "A", "C" ID cards
	IssuingState   string // ISO 3166 alpha-3, e.g. "UZB"
	Surname        string
	GivenNames     string
	DocumentNumber string
	Nationality    string
	BirthDate      string // YYYY-MM-DD
	Sex            string // "M", "F" or "" (unspecified)
	ExpiryDate     string // YYYY-MM-DD
}

// FullName is "GIVEN NAMES SURNAME", as other documents usually write it.
func (m *MRZ) FullName() string { return strings.TrimSpace(m.GivenNames + " " + m.Surname) }

// FindMRZ looks for a valid machine-readable zone in OCR text. now is used
// to pick the century of two-digit years.
func FindMRZ(text string, now time.Time) (*MRZ, error) {
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		if c := cleanMRZLine(l); c != "" {
			lines = append(lines, c)
		}
	}
	var lastErr error = fmt.Errorf("no machine-readable zone found")
	// TD3: two consecutive 44-character lines, the first starting with P.
	for i := 0; i+1 < len(lines); i++ {
		a, b := fit(lines[i], 44), fit(lines[i+1], 44)
		if a == "" || b == "" || a[0] != 'P' {
			continue
		}
		m, err := parseTD3(a, b, now)
		if err == nil {
			return m, nil
		}
		lastErr = err
	}
	// TD1: three consecutive 30-character lines.
	for i := 0; i+2 < len(lines); i++ {
		a, b, c := fit(lines[i], 30), fit(lines[i+1], 30), fit(lines[i+2], 30)
		if a == "" || b == "" || c == "" || !strings.ContainsRune("IAC", rune(a[0])) {
			continue
		}
		m, err := parseTD1(a, b, c, now)
		if err == nil {
			return m, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// cleanMRZLine keeps a line only if it looks like MRZ text, normalising the
// usual OCR confusions of the filler character.
func cleanMRZLine(l string) string {
	l = strings.ToUpper(strings.TrimSpace(l))
	r := strings.NewReplacer(" ", "", "«", "<<", "‹", "<", "〈", "<", "‘", "", "'", "")
	l = r.Replace(l)
	if len(l) < 28 || strings.Count(l, "<") < 2 {
		return ""
	}
	for _, c := range l {
		if !(c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '<') {
			return ""
		}
	}
	return l
}

// fit pads or trims a line to n characters when it is close to that length
// (OCR often drops or adds a filler at the end).
func fit(l string, n int) string {
	switch {
	case len(l) == n:
		return l
	case len(l) < n && len(l) >= n-3:
		return l + strings.Repeat("<", n-len(l))
	case len(l) > n && len(l) <= n+2 && strings.Trim(l[n:], "<") == "":
		return l[:n]
	}
	return ""
}

func parseTD3(a, b string, now time.Time) (*MRZ, error) {
	m := &MRZ{Format: "TD3", DocumentCode: strings.TrimRight(a[0:2], "<"), IssuingState: alpha(a[2:5])}
	m.Surname, m.GivenNames = names(a[5:44])
	num, err := checked("document number", b[0:9], b[9])
	if err != nil {
		return nil, err
	}
	m.DocumentNumber = strings.TrimRight(num, "<")
	m.Nationality = alpha(b[10:13])
	birth, err := checked("birth date", digits(b[13:19]), b[19])
	if err != nil {
		return nil, err
	}
	m.Sex = sex(b[20])
	expiry, err := checked("expiry date", digits(b[21:27]), b[27])
	if err != nil {
		return nil, err
	}
	// Optional data (personal number) and the composite check digit.
	opt := b[28:42]
	if b[42] != '<' {
		if _, err := checked("personal number", opt, b[42]); err != nil {
			return nil, err
		}
	}
	composite := b[0:10] + digits(b[13:20]) + digits(b[21:28]) + opt + string(b[42])
	if _, err := checked("composite", composite, b[43]); err != nil {
		return nil, err
	}
	if m.BirthDate, err = mrzDate(birth, now, false); err != nil {
		return nil, err
	}
	if m.ExpiryDate, err = mrzDate(expiry, now, true); err != nil {
		return nil, err
	}
	return m, nil
}

func parseTD1(a, b, c string, now time.Time) (*MRZ, error) {
	m := &MRZ{Format: "TD1", DocumentCode: strings.TrimRight(a[0:2], "<"), IssuingState: alpha(a[2:5])}
	num, err := checked("document number", a[5:14], a[14])
	if err != nil {
		return nil, err
	}
	m.DocumentNumber = strings.TrimRight(num, "<")
	birth, err := checked("birth date", digits(b[0:6]), b[6])
	if err != nil {
		return nil, err
	}
	m.Sex = sex(b[7])
	expiry, err := checked("expiry date", digits(b[8:14]), b[14])
	if err != nil {
		return nil, err
	}
	m.Nationality = alpha(b[15:18])
	composite := a[5:30] + digits(b[0:7]) + digits(b[8:15]) + b[18:29]
	if _, err := checked("composite", composite, b[29]); err != nil {
		return nil, err
	}
	m.Surname, m.GivenNames = names(c)
	if m.BirthDate, err = mrzDate(birth, now, false); err != nil {
		return nil, err
	}
	if m.ExpiryDate, err = mrzDate(expiry, now, true); err != nil {
		return nil, err
	}
	return m, nil
}

// checked verifies field against its check digit. OCR may read the digit
// 0 as the letter O; numeric check digits are repaired before comparing.
func checked(name, field string, digit byte) (string, error) {
	d := digit
	if d == 'O' || d == 'Q' || d == 'D' {
		d = '0'
	}
	if d == '<' {
		d = '0'
	}
	if d < '0' || d > '9' {
		return "", fmt.Errorf("%s check digit unreadable", name)
	}
	if checkDigit(field) != int(d-'0') {
		return "", fmt.Errorf("%s check digit does not match", name)
	}
	return field, nil
}

// checkDigit is the ICAO 9303 7-3-1 checksum.
func checkDigit(s string) int {
	w := [3]int{7, 3, 1}
	sum := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		v := 0
		switch {
		case c >= '0' && c <= '9':
			v = int(c - '0')
		case c >= 'A' && c <= 'Z':
			v = int(c-'A') + 10
		}
		sum += v * w[i%3]
	}
	return sum % 10
}

// digits repairs letters OCR commonly reads in numeric fields.
func digits(s string) string {
	return strings.NewReplacer("O", "0", "Q", "0", "D", "0", "I", "1", "L", "1", "Z", "2", "S", "5", "B", "8", "G", "6").Replace(s)
}

// alpha repairs digits OCR commonly reads in letter-only fields.
func alpha(s string) string {
	return strings.TrimRight(strings.NewReplacer("0", "O", "1", "I", "5", "S", "8", "B").Replace(s), "<")
}

func names(s string) (surname, given string) {
	s = strings.TrimRight(alpha(s), "<")
	parts := strings.SplitN(s, "<<", 2)
	surname = strings.Join(strings.Fields(strings.ReplaceAll(parts[0], "<", " ")), " ")
	if len(parts) == 2 {
		given = strings.Join(strings.Fields(strings.ReplaceAll(parts[1], "<", " ")), " ")
	}
	return surname, given
}

func sex(c byte) string {
	switch c {
	case 'M', 'F':
		return string(c)
	}
	return ""
}

// mrzDate turns YYMMDD into YYYY-MM-DD. Birth dates are in the past; expiry
// dates can be up to 20 years ahead.
func mrzDate(s string, now time.Time, expiry bool) (string, error) {
	t, err := time.Parse("060102", s)
	if err != nil {
		return "", fmt.Errorf("invalid date %q", s)
	}
	yy := t.Year() % 100
	century := now.Year() / 100 * 100
	year := century + yy
	if expiry {
		if year < now.Year()-30 {
			year += 100
		}
	} else if year > now.Year() {
		year -= 100
	}
	return fmt.Sprintf("%04d-%02d-%02d", year, t.Month(), t.Day()), nil
}
