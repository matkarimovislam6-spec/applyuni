package requirements

import (
	"encoding/json"
	"testing"
)

var rules = json.RawMessage(`{
  "documents": [
    {"type":"passport","title":"Passport","degrees":["bachelor","master"]},
    {"type":"school_diploma","title":"Diploma","degrees":["bachelor"]},
    {"type":"university_year_certificate","title":"Uni year","degrees":["bachelor"],"condition":"school_years_lt_12"},
    {"type":"english_certificate","title":"IELTS","condition":"english_taught"},
    {"type":"isee_family_documents","title":"ISEE","condition":"wants_scholarship"}
  ]
}`)

func types(cl *Checklist) map[string]string {
	m := map[string]string{}
	for _, it := range cl.Items {
		m[it.Type] = it.Status
	}
	return m
}

func TestUzbekBachelorWith11Years(t *testing.T) {
	cl, err := Build(rules, nil, Profile{SchoolYears: 11, WantsScholarship: true}, Program{Degree: "bachelor", Language: "en"},
		[]UploadedDoc{{ID: "d1", Type: "passport", Status: "accepted"}})
	if err != nil {
		t.Fatal(err)
	}
	got := types(cl)
	for _, want := range []string{"passport", "school_diploma", "university_year_certificate", "english_certificate", "isee_family_documents"} {
		if _, ok := got[want]; !ok {
			t.Errorf("missing %s", want)
		}
	}
	if got["passport"] != "accepted" || cl.Complete {
		t.Errorf("unexpected status: %v complete=%v", got, cl.Complete)
	}
	if len(cl.Warnings) == 0 {
		t.Error("expected 12-year schooling warning")
	}
}

func TestMasterSkipsBachelorOnlyDocs(t *testing.T) {
	cl, _ := Build(rules, nil, Profile{SchoolYears: 11}, Program{Degree: "master", Language: "it"}, nil)
	got := types(cl)
	if _, ok := got["school_diploma"]; ok {
		t.Error("master should not need school diploma")
	}
	if _, ok := got["english_certificate"]; ok {
		t.Error("Italian-taught programme should not require English certificate")
	}
}
