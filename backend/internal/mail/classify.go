// Package mail understands the emails universities send to a student's
// platform address: verification codes, "we received your application",
// requests for more documents, and the admission decision. Classification
// is phrase-based and conservative: an email only counts as a decision when
// it uses the unmistakable wording universities use for one; anything
// unclear stays "other" and is shown to the student as is.
package mail

import (
	"regexp"
	"strings"
)

type Kind string

const (
	Verification   Kind = "verification"
	Received       Kind = "received"
	ActionRequired Kind = "action_required"
	Admitted       Kind = "admitted"
	NotAdmitted    Kind = "not_admitted"
	Waitlisted     Kind = "waitlisted"
	Other          Kind = "other"
)

// IsDecision reports whether the email carries the university's answer.
func (k Kind) IsDecision() bool { return k == Admitted || k == NotAdmitted || k == Waitlisted }

// Rules are checked in order; the first match wins. Rejections come before
// admissions because "not admitted" contains "admitted".
var rules = []struct {
	kind Kind
	re   *regexp.Regexp
}{
	{Verification, re(`verification code|verify your e-?mail|confirm your e-?mail|one-time (pass)?code|codice di (verifica|conferma)|bestätigungscode|best(ä|ae)tigen sie ihre e-?mail`)},
	{NotAdmitted, re(`regret to inform|unable to offer you|not (been )?(admitted|accepted|selected)|unfortunately,? (we|your application)|non (sei |è )?(stat[oa] )?ammess[oa]|siamo spiacenti|non idone[oa]|leider (können|koennen) wir|ablehnungsbescheid|ablehnung ihres antrags`)},
	{Waitlisted, re(`wait[- ]?list|lista d'attesa|warteliste|nachrückverfahren`)},
	{Admitted, re(`congratulations|pleased to (inform|offer)|offer of admission|you have been (admitted|accepted)|admission decision: admitted|sei stat[oa] ammess[oa]|risulti ammess[oa]|idone[oa] e ammess[oa]|zulassungsbescheid|sie wurden zugelassen|zulassung zum studium`)},
	{ActionRequired, re(`missing document|additional (document|information)|incomplete application|action required|please (upload|submit|provide)|documentazione mancante|integrazione (dei )?documenti|fehlende unterlagen|unterlagen nachreichen`)},
	{Received, re(`(we have|we've) received your application|application (has been )?received|application submitted successfully|domanda (è stata )?(ricevuta|inviata)|eingangsbestätigung|antrag (ist )?eingegangen`)},
}

func re(s string) *regexp.Regexp { return regexp.MustCompile(`(?i)` + s) }

// Classify reads the subject and body of one email.
func Classify(subject, body string) Kind {
	text := strings.Join(strings.Fields(subject+"\n"+body), " ")
	for _, r := range rules {
		if r.re.MatchString(text) {
			return r.kind
		}
	}
	return Other
}

var addrRe = regexp.MustCompile(`[A-Za-z0-9._%+-]+@([A-Za-z0-9.-]+\.[A-Za-z]{2,})`)

// Domain returns the lowercased domain of an address like
// "Admissions <noreply@polimi.it>".
func Domain(addr string) string {
	m := addrRe.FindStringSubmatch(addr)
	if m == nil {
		return ""
	}
	return strings.ToLower(m[1])
}

// SameOrganisation reports whether a sender domain belongs to a university
// website: "apply.polimi.it" and "polimi.it" match "https://www.polimi.it".
func SameOrganisation(senderDomain, website string) bool {
	site := strings.ToLower(website)
	site = strings.TrimPrefix(strings.TrimPrefix(site, "https://"), "http://")
	site = strings.TrimPrefix(strings.SplitN(site, "/", 2)[0], "www.")
	if site == "" || senderDomain == "" {
		return false
	}
	return senderDomain == site || strings.HasSuffix(senderDomain, "."+site) || strings.HasSuffix(site, "."+senderDomain)
}
