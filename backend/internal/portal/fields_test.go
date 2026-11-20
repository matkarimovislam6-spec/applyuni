package portal

import "testing"

func TestBuildFieldsUsesPassportDetails(t *testing.T) {
	f := BuildFields("islom_m", "s-1@inbox", "u", "p", map[string]any{
		"legal_first_name": "ISLOM", "legal_last_name": "MATKARIMOV", "date_of_birth": "2004-03-01", "sex": "M",
	}, "CS", "master")
	want := map[string]string{
		"student.first_name": "ISLOM", "student.last_name": "MATKARIMOV", "student.full_name": "ISLOM MATKARIMOV",
		"student.birth_day": "01", "student.birth_month": "03", "student.birth_year": "2004", "profile.sex": "M",
	}
	for k, v := range want {
		if f[k] != v {
			t.Errorf("%s = %q, want %q", k, f[k], v)
		}
	}
	// Without passport details the account name is still used.
	f = BuildFields("Nilufar Karimova", "e", "u", "p", nil, "CS", "master")
	if f["student.first_name"] != "Nilufar" || f["student.birth_day"] != "" {
		t.Errorf("fallback: %v", f)
	}
}
