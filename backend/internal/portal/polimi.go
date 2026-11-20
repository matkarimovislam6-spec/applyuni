package portal

// Politecnico di Milano — Online Services (aunicalogin / aunicaaccount2).
// Mapped read-only from the live site on 2026-11-20. Nothing here has been
// filled in or submitted on the real portal.
//
// What we know (official page "Application", foreign qualification, MSc):
//   A. Register online → a Person Code + password (4-step registration)
//   B. Fill out the online application form (service 239)
//   C. Pay the application fee (pagoPA); after paying, documents are locked
// Polimi asks applicants to use a personal email and to check the Online
// Services personally, and discourages unauthorised agents.
//
// Registration step 1/4 "Identification Data" is mapped below. Steps 2–4,
// the application form and the payment can only be mapped by going through
// them with a real person's consent and data, so PolimiFlow is incomplete
// and is NOT in Known.

const polimiRegisterURL = "https://aunicalogin.polimi.it/aunicalogin/getservizio.xml?id_servizio=376&lang=EN"

// Country codes in Polimi's own lists.
const polimiUzbekistan = "132" // "UZBEKISTAN" (country of birth) and "UZBEK" (citizenship)

// PolimiRegistrationStep1 fills "Identification Data". The sign-up page has
// a per-session URL, so it is reached by clicking "Sign up" on the login page.
var PolimiRegistrationStep1 = []Step{
	{Kind: Goto, Name: "Open Online Services", Value: polimiRegisterURL},
	{Kind: Click, Name: "Sign up", Selector: `a[href*="evn_REGISTRATI"]`},
	{Kind: Expect, Name: "Registration step 1/4", Selector: "body", Contains: "registration step 1/4"},
	{Kind: Fill, Name: "Surname", Selector: "#input_cognome", Value: "{{student.last_name}}"},
	{Kind: Fill, Name: "Name", Selector: "#input_nome", Value: "{{student.first_name}}"},
	{Kind: Tick, Name: "Sex", Selector: `input[name="input_sesso"][value="{{profile.sex}}"]`},
	{Kind: Fill, Name: "Birth day", Selector: `input[name="input_datanasc_gg"]`, Value: "{{student.birth_day}}"},
	{Kind: Fill, Name: "Birth month", Selector: `input[name="input_datanasc_mm"]`, Value: "{{student.birth_month}}"},
	{Kind: Fill, Name: "Birth year", Selector: `input[name="input_datanasc_aaaa"]`, Value: "{{student.birth_year}}"},
	{Kind: Choose, Name: "Country of birth", Selector: "#input_nas_stato", Value: polimiUzbekistan},
	{Kind: Choose, Name: "Citizenship", Selector: "#input_cittadin", Value: polimiUzbekistan},
	// Tax code (#input_codfiscale) is only for Italian citizens/residents: left empty.
	{Kind: Fill, Name: "Personal email", Selector: "#input_email", Value: "{{student.email}}"},
	{Kind: Click, Name: "Next", Selector: `button[name="evn_avanti"]`},
	// Steps 2–4: not mapped yet.
}

// PolimiSelectors lists every selector the mapped steps rely on, so a live
// check can tell us when Polimi changes its pages.
func PolimiSelectors() []string {
	var out []string
	for _, s := range PolimiRegistrationStep1 {
		if s.Selector != "" && s.Selector != "body" && s.Kind != Click && s.Kind != Tick {
			out = append(out, s.Selector)
		}
	}
	return append(out, `input[name="input_sesso"]`, `button[name="evn_avanti"]`)
}
