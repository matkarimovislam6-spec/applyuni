package mail

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		subject, body string
		want          Kind
	}{
		{"Your verification code", "Your code is 482913", Verification},
		{"Codice di verifica", "Il tuo codice è 112233", Verification},
		{"Application decision", "We regret to inform you that you have not been admitted.", NotAdmitted},
		{"Decision", "Unfortunately, we are unable to offer you a place.", NotAdmitted},
		{"Esito", "Siamo spiacenti: non sei stato ammesso al corso.", NotAdmitted},
		{"Ihr Antrag", "Leider können wir Ihnen keinen Studienplatz anbieten.", NotAdmitted},
		{"Congratulations!", "We are pleased to offer you admission to Computer Science.", Admitted},
		{"Zulassungsbescheid", "Sie wurden zugelassen zum Wintersemester.", Admitted},
		{"Esito della selezione", "Sei stato ammesso al corso di laurea magistrale.", Admitted},
		{"Update", "You have been placed on the waitlist.", Waitlisted},
		{"Incomplete application", "Please upload your missing documents by Friday.", ActionRequired},
		{"Confirmation", "We have received your application for Fall 2027.", Received},
		{"Newsletter", "Join our open day on campus!", Other},
	}
	for _, c := range cases {
		if got := Classify(c.subject, c.body); got != c.want {
			t.Errorf("%q / %q: got %s, want %s", c.subject, c.body, got, c.want)
		}
	}
}

func TestSameOrganisation(t *testing.T) {
	if d := Domain("Admissions <noreply@Apply.Polimi.it>"); d != "apply.polimi.it" || !SameOrganisation(d, "https://www.polimi.it") {
		t.Errorf("polimi subdomain should match (%s)", d)
	}
	if SameOrganisation("unimi.it", "https://www.polimi.it") {
		t.Error("different university matched")
	}
	if SameOrganisation("evilpolimi.it", "https://www.polimi.it") {
		t.Error("lookalike domain matched")
	}
}
