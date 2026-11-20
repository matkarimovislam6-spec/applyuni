package portal

// DreamApply (Dream Group, Tallinn): a hosted admissions system used by many
// European universities. In our catalogue: University of Padova
// (apply.unipd.it). Mapped read-only from the live site on 2026-11-20.
//
// What we know:
//   - Every DreamApply university sends applicants to one central login,
//     DreamID (id.dreamapply.com/login?client=<university>). One DreamID
//     account can be used at every DreamApply university.
//   - Sign-in starts with an email address and a "Continue" button; Google,
//     Facebook, LinkedIn and passkey sign-in are offered too.
//
// The pages after "Continue" (account creation, email confirmation, the
// application form, document uploads, submission) can only be seen with a
// real account, so there is no DreamApply Flow yet. Next step: an approved
// applicant's dry run through Agent, whose learned steps become the Flow.

const (
	dreamIDLogin = "https://id.dreamapply.com/login"
	dreamIDEmail = "#email"
)

// DreamApplyPortals maps our portal ids to each university's DreamApply site.
var DreamApplyPortals = map[string]string{
	"unipd": "https://apply.unipd.it/",
}
