// Package i18n translates the text the backend produces (checklists,
// matching explanations, deadline notes, errors) into Uzbek and Russian.
//
// Catalogs (uz.json, ru.json) map an English text to its translation. A key
// may be a full sentence, or a sentence pattern with Go verbs (%s, %d, %.1f,
// %%) wherever a value varies:
//
//	"Your %s is %d%% of the scale; for a %s programme %d%%+ is strong."
//	  -> "Sizning %[1]s: shkalaning %[2]s%% ..."
//
// Translations refer to the captured values with %s or %[n]s (reordering is
// allowed). Captured values are themselves translated when the catalog knows
// them (e.g. "bachelor's GPA"), and English month names in dates are
// converted. Unknown text is returned unchanged (English).
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
)

const (
	EN = "en"
	UZ = "uz"
	RU = "ru"
)

//go:embed uz.json ru.json
var files embed.FS

type pattern struct {
	re    *regexp.Regexp
	tmpl  string
	score int // literal length: more specific patterns are tried first
}

type catalog struct {
	exact    map[string]string
	patterns []pattern
}

var (
	once     sync.Once
	catalogs map[string]*catalog
	loadErr  error
)

var verbRe = regexp.MustCompile(`%(\[\d+\])?[-+# 0]*\d*(\.\d+)?[sdvfq%]`)

func compile(key string) (*regexp.Regexp, int) {
	var b strings.Builder
	b.WriteString("^")
	literal := 0
	last := 0
	for _, m := range verbRe.FindAllStringIndex(key, -1) {
		lit := key[last:m[0]]
		b.WriteString(regexp.QuoteMeta(lit))
		literal += len(lit)
		verb := key[m[0]:m[1]]
		switch verb[len(verb)-1] {
		case '%':
			b.WriteString("%")
			literal++
		case 'd':
			b.WriteString(`(-?\d+)`)
		case 'f':
			b.WriteString(`(-?[\d.]+)`)
		case 'q':
			b.WriteString(`"(.*?)"`)
		default:
			b.WriteString(`(.+?)`)
		}
		last = m[1]
	}
	lit := key[last:]
	b.WriteString(regexp.QuoteMeta(lit))
	literal += len(lit)
	b.WriteString("$")
	return regexp.MustCompile(b.String()), literal
}

func load() {
	catalogs = map[string]*catalog{}
	for _, lang := range []string{UZ, RU} {
		raw, err := files.ReadFile(lang + ".json")
		if err != nil {
			loadErr = err
			return
		}
		var m map[string]string
		if err := json.Unmarshal(raw, &m); err != nil {
			loadErr = fmt.Errorf("%s.json: %w", lang, err)
			return
		}
		c := &catalog{exact: map[string]string{}}
		for k, v := range m {
			if verbRe.MatchString(k) {
				re, score := compile(k)
				c.patterns = append(c.patterns, pattern{re: re, tmpl: v, score: score})
			} else {
				c.exact[k] = v
			}
		}
		sort.SliceStable(c.patterns, func(i, j int) bool { return c.patterns[i].score > c.patterns[j].score })
		catalogs[lang] = c
	}
}

// LoadError reports a broken catalog file (checked by tests and at start).
func LoadError() error {
	once.Do(load)
	return loadErr
}

// Lookup translates s, reporting whether a translation was found.
func Lookup(lang, s string) (string, bool) {
	return lookup(lang, s, 0)
}

func lookup(lang, s string, depth int) (string, bool) {
	once.Do(load)
	c := catalogs[lang]
	if c == nil || s == "" {
		return s, lang == EN || s == ""
	}
	if t, ok := c.exact[s]; ok {
		return t, true
	}
	if depth > 2 {
		return s, false
	}
	for _, p := range c.patterns {
		m := p.re.FindStringSubmatch(s)
		if m == nil {
			continue
		}
		args := make([]any, len(m)-1)
		for i, a := range m[1:] {
			if t, ok := lookup(lang, a, depth+1); ok {
				args[i] = t
			} else {
				args[i] = months(lang, a)
			}
		}
		return fmt.Sprintf(p.tmpl, args...), true
	}
	return s, false
}

// T translates s, falling back to the English text.
func T(lang, s string) string {
	t, _ := Lookup(lang, s)
	return t
}

// TP translates an optional string in place.
func TP(lang string, s *string) {
	if s != nil {
		*s = T(lang, *s)
	}
}

var monthNames = map[string][12]string{
	UZ: {"yan", "fev", "mar", "apr", "may", "iyun", "iyul", "avg", "sen", "okt", "noy", "dek"},
	RU: {"янв.", "февр.", "мар.", "апр.", "мая", "июн.", "июл.", "авг.", "сент.", "окт.", "нояб.", "дек."},
}
var monthRe = regexp.MustCompile(`\b(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)\b`)
var monthIdx = map[string]int{"Jan": 0, "Feb": 1, "Mar": 2, "Apr": 3, "May": 4, "Jun": 5, "Jul": 6, "Aug": 7, "Sep": 8, "Oct": 9, "Nov": 10, "Dec": 11}

// months converts English month abbreviations in dates like "1 Dec 2026".
func months(lang, s string) string {
	names, ok := monthNames[lang]
	if !ok {
		return s
	}
	return monthRe.ReplaceAllStringFunc(s, func(m string) string { return names[monthIdx[m]] })
}

// FromRequest picks the language: X-Lang header, then Accept-Language, else English.
func FromRequest(r *http.Request) string {
	for _, v := range []string{r.Header.Get("X-Lang"), r.Header.Get("Accept-Language")} {
		v = strings.ToLower(strings.TrimSpace(v))
		switch {
		case strings.HasPrefix(v, "uz"):
			return UZ
		case strings.HasPrefix(v, "ru"):
			return RU
		case strings.HasPrefix(v, "en"):
			return EN
		}
	}
	return EN
}
