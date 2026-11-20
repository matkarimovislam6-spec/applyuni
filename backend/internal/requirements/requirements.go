// Package requirements turns country rules + programme requirements + the
// student's profile into a concrete document checklist.
package requirements

import (
	"encoding/json"
	"slices"
)

type DocRule struct {
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Degrees     []string `json:"degrees,omitempty"`
	Condition   string   `json:"condition,omitempty"`
}

type Step struct {
	Key   string `json:"key"`
	Title string `json:"title"`
}

type CountryRules struct {
	Documents []DocRule `json:"documents"`
	Steps     []Step    `json:"steps"`
}

type ProgramRequirements struct {
	ExtraDocuments []DocRule `json:"extra_documents"`
}

// Profile is the subset of the student profile the rules depend on.
type Profile struct {
	SchoolYears          int    `json:"school_years"`
	DocumentsLanguage    string `json:"documents_language"` // "en", "uz", "ru"
	WantsScholarship     bool   `json:"wants_scholarship"`
	EnglishTest          string `json:"english_test"`
	EnglishScore         string `json:"english_score"`
	GPA                  string `json:"gpa"`
	HasCompletedUniYears int    `json:"completed_university_years"`
}

type Program struct {
	Degree   string
	Language string
}

type Item struct {
	DocRule
	Status     string `json:"status"` // missing | uploaded | analyzing | accepted | rejected
	DocumentID string `json:"document_id,omitempty"`
}

type UploadedDoc struct {
	ID     string
	Type   string
	Status string
}

type Checklist struct {
	Items    []Item   `json:"items"`
	Steps    []Step   `json:"steps"`
	Warnings []string `json:"warnings"`
	Complete bool     `json:"complete"`
}

func Build(countryRules, programReq json.RawMessage, prof Profile, prog Program, docs []UploadedDoc) (*Checklist, error) {
	var cr CountryRules
	if err := json.Unmarshal(countryRules, &cr); err != nil {
		return nil, err
	}
	var pr ProgramRequirements
	if len(programReq) > 0 {
		if err := json.Unmarshal(programReq, &pr); err != nil {
			return nil, err
		}
	}

	cl := &Checklist{Steps: cr.Steps, Warnings: []string{}, Items: []Item{}}
	rules := append(append([]DocRule{}, cr.Documents...), pr.ExtraDocuments...)
	seen := map[string]bool{}
	for _, r := range rules {
		if seen[r.Type] {
			continue
		}
		if len(r.Degrees) > 0 && !slices.Contains(r.Degrees, prog.Degree) {
			continue
		}
		if !conditionHolds(r.Condition, prof, prog) {
			continue
		}
		seen[r.Type] = true
		cl.Items = append(cl.Items, Item{DocRule: r, Status: statusFor(r.Type, docs, &Item{})})
	}
	// attach document ids
	for i := range cl.Items {
		cl.Items[i].Status = statusFor(cl.Items[i].Type, docs, &cl.Items[i])
	}

	if prog.Degree == "bachelor" && prof.SchoolYears > 0 && prof.SchoolYears < 12 && prof.HasCompletedUniYears < 1 {
		cl.Warnings = append(cl.Warnings, "You have fewer than 12 years of school. Italian universities will require proof of at least one completed university year (or a recognised foundation year) before you can enrol in a bachelor's programme.")
	}
	if prog.Language == "en" && prof.EnglishTest == "" {
		cl.Warnings = append(cl.Warnings, "No English test in your profile yet. Book IELTS/TOEFL early — results take up to 2 weeks.")
	}

	cl.Complete = true
	for _, it := range cl.Items {
		if it.Status != "accepted" {
			cl.Complete = false
		}
	}
	return cl, nil
}

func conditionHolds(cond string, p Profile, prog Program) bool {
	switch cond {
	case "":
		return true
	case "school_years_lt_12":
		return p.SchoolYears > 0 && p.SchoolYears < 12
	case "english_taught":
		return prog.Language == "en"
	case "wants_scholarship":
		return p.WantsScholarship
	case "documents_not_in_english":
		return p.DocumentsLanguage != "en"
	default:
		return true // unknown condition: be safe and ask for the document
	}
}

// statusFor picks the best status among uploaded documents of this type.
func statusFor(docType string, docs []UploadedDoc, it *Item) string {
	rank := map[string]int{"rejected": 1, "uploaded": 2, "analyzing": 3, "accepted": 4}
	best := "missing"
	for _, d := range docs {
		if d.Type == docType && rank[d.Status] > rank[best] {
			best = d.Status
			it.DocumentID = d.ID
		}
	}
	return best
}
