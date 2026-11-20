package verify

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Names of Uzbek students appear in several spellings across documents:
// Cyrillic on old school certificates, Uzbek Latin on new ones, Russian-style
// or ICAO transliteration in passports, English on translations. For example
// "Ислом Маткаримов", "Islom Matkarimov" and "Islam Matkarimov" are one
// person. NamesMatch compares the sound skeleton of each name part, ignores
// order and patronymics, and tolerates one small spelling difference per part.

var cyr = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "yo", 'ж': "j", 'з': "z",
	'и': "i", 'й': "y", 'к': "k", 'л': "l", 'м': "m", 'н': "n", 'о': "o", 'п': "p", 'р': "r",
	'с': "s", 'т': "t", 'у': "u", 'ф': "f", 'х': "x", 'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "sh",
	'ъ': "", 'ы': "i", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",
	'ў': "o", 'қ': "q", 'ғ': "g", 'ҳ': "h",
}

// Words that mark a patronymic ("Islom Bahodir o'g'li") or are noise.
var patronymicWords = map[string]bool{"ugli": true, "ogli": true, "uli": true, "kizi": true, "qizi": true}

var stripMarks = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// skeleton reduces one name part to a spelling-independent form.
func skeleton(word string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(word) {
		if s, ok := cyr[r]; ok {
			b.WriteString(s)
		} else {
			b.WriteRune(r)
		}
	}
	s, _, _ := transform.String(stripMarks, b.String())
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' {
			return r
		}
		return -1 // apostrophes (o' g' o‘ oʻ), hyphens, dots
	}, s)
	for _, p := range [][2]string{
		{"dzh", "j"}, {"dj", "j"}, {"zh", "j"}, {"kh", "x"}, {"h", "x"}, {"gh", "g"}, {"q", "k"},
		{"ck", "k"}, {"w", "v"}, {"ph", "f"}, {"y", "i"}, {"o", "a"}, {"e", "i"},
	} {
		s = strings.ReplaceAll(s, p[0], p[1])
	}
	// collapse doubled letters: "Muhammad" / "Muhamad"
	var out []byte
	for i := 0; i < len(s); i++ {
		if i == 0 || s[i] != s[i-1] {
			out = append(out, s[i])
		}
	}
	return string(out)
}

// nameParts splits a full name into comparable parts, dropping patronymics.
func nameParts(full string) []string {
	var parts []string
	for _, w := range strings.FieldsFunc(full, func(r rune) bool { return unicode.IsSpace(r) || r == ',' || r == '/' }) {
		sk := skeleton(w)
		if sk == "" || patronymicWords[sk] || patronymicWords[strings.ReplaceAll(sk, "a", "u")] {
			continue
		}
		// Russian-style patronymics: -ovich / -evich / -ovna / -evna
		if len(sk) > 6 && (strings.HasSuffix(sk, "avich") || strings.HasSuffix(sk, "ivich") || strings.HasSuffix(sk, "avna") || strings.HasSuffix(sk, "ivna")) {
			continue
		}
		parts = append(parts, sk)
	}
	return parts
}

// NamesMatch reports whether two written names belong to the same person.
// Every part of the shorter name must match a different part of the longer
// one, and at least two parts must match when both names have two or more.
func NamesMatch(a, b string) bool {
	pa, pb := nameParts(a), nameParts(b)
	if len(pa) == 0 || len(pb) == 0 {
		return false
	}
	if len(pa) > len(pb) {
		pa, pb = pb, pa
	}
	used := make([]bool, len(pb))
	matched := 0
	for _, x := range pa {
		for j, y := range pb {
			if !used[j] && close(x, y) {
				used[j] = true
				matched++
				break
			}
		}
	}
	// Two parts must match whenever either name has two (first + last); one
	// extra part (a middle name on one document only) is tolerated.
	need := min(2, len(pb))
	return matched >= need && matched >= len(pa)-1
}

// close allows one edit for short parts, two for long ones.
func close(a, b string) bool {
	limit := 1
	if min(len(a), len(b)) >= 9 {
		limit = 2
	}
	return levenshtein(a, b) <= limit
}

func levenshtein(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// Known is what we already know about the student, from their passport or
// profile, so a reader can look for it on other documents.
type Known struct {
	FullName  string // as in the passport
	BirthDate string // YYYY-MM-DD
}

// NameAppearsIn reports whether every part of name (at least two parts for
// a multi-part name) is written somewhere in text, in any of the spellings
// NamesMatch accepts.
func NameAppearsIn(name, text string) bool {
	want := nameParts(name)
	if len(want) == 0 {
		return false
	}
	var have []string
	for _, w := range strings.FieldsFunc(text, func(r rune) bool {
		return !(unicode.IsLetter(r) || r == '\'' || r == '‘' || r == 'ʻ' || r == '’')
	}) {
		if sk := skeleton(w); len(sk) >= 2 {
			have = append(have, sk)
		}
	}
	for _, p := range want {
		found := false
		for _, h := range have {
			if close(p, h) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
