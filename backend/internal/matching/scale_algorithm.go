package matching

// Weighted grade scoring.
//
// A GPA means different things depending on where it was earned and where it
// is sent. 83% from a less selective university is not the same signal to a
// top programme as 80% from a top university. The score therefore combines:
//
//  1. pct          the GPA as a fraction of its own scale (3.33/4 = 0.83)
//  2. origin tier  how selective the student's university is (1 = world top … 4)
//  3. target tier  how selective the programme is (1 = very selective … 3)
//
// Step 1: adjusted = pct × (1 + originBonus × targetWeight), capped at 1.
//         originBonus comes from the origin tier; targetWeight makes the origin
//         count more for selective programmes and less for moderate ones.
// Step 2: the adjusted grade is placed between the programme's realistic floor
//         and its "strong" level, then smoothed (smoothstep) so small
//         differences near the edges don't swing the score.
//
// IMPORTANT: the tiers below are editorial starting values, not official
// rankings. They are meant to be tuned against real admission outcomes.
// Programme minimums published by universities (Meta.MinGPAPct) are checked
// separately against the raw GPA, never the adjusted one.

import (
	"fmt"
	"math"
	"strings"
)

// ---------------- origin tiers (editorial, edit freely) ----------------

// originBonus per tier: how much a grade from that tier is worth relative to
// a typical national university (tier 3 = 0).
var originBonus = map[int]float64{
	1: 0.12,  // internationally top-ranked universities
	2: 0.06,  // strong, selective or internationally accredited universities
	3: 0.00,  // established national universities, and the default
	4: -0.06, // less selective institutions
}

var originTierName = map[int]string{
	1: "a top-ranked university",
	2: "a selective university",
	3: "a national university",
	4: "a less selective institution",
}

// originTiers maps lower-case name fragments to a tier. The first (longest)
// matching fragment wins. Unknown universities get tier 3 (neutral).
var originTiers = map[string]int{
	// world top (examples; extend as needed)
	"massachusetts institute of technology": 1, "stanford": 1, "harvard": 1, "oxford": 1, "cambridge": 1,
	"eth zurich": 1, "imperial college": 1, "carnegie mellon": 1,
	// catalogue universities and similar
	"technical university of munich": 1, "georgia institute of technology": 1, "georgia tech": 1,
	"university of michigan": 1, "university of illinois": 1, "texas at austin": 1,
	"politecnico di milano": 2, "bocconi": 2, "rwth": 2, "purdue": 2, "university of bologna": 2,
	"sapienza": 2, "university of padova": 2, "politecnico di torino": 2, "university of pisa": 2,
	"university of trento": 2, "university of mannheim": 2, "university of freiburg": 2,
	"university of bonn": 2, "northeastern": 2, "arizona state": 3, "university of milan": 2,
	// Uzbekistan
	"westminster international university": 2, "wiut": 2,
	"inha university in tashkent": 2, "inha": 2,
	"turin polytechnic university in tashkent": 2, "ttpu": 2,
	"new uzbekistan university":                       2,
	"national university of uzbekistan":               2,
	"tashkent university of information technologies": 3, "tuit": 3,
	"tashkent state technical university":    3,
	"tashkent state university of economics": 3,
	"samarkand state university":             3,
}

// OriginTier finds the tier of the student's university from its name.
// known is false when the name didn't match anything (tier 3 is assumed).
func OriginTier(university string) (tier int, known bool) {
	name := strings.ToLower(strings.TrimSpace(university))
	if name == "" {
		return 3, false
	}
	best, bestLen := 3, 0
	for frag, t := range originTiers {
		if strings.Contains(name, frag) && len(frag) > bestLen {
			best, bestLen = t, len(frag)
		}
	}
	return best, bestLen > 0
}

// ---------------- target tiers ----------------

type targetLevel struct {
	floor, strong float64 // adjusted-grade levels: realistic minimum, and "strong"
	originWeight  float64 // how much the origin university counts for this programme
	name          string
}

var targetLevels = map[int]targetLevel{
	1: {floor: 0.72, strong: 0.88, originWeight: 1.25, name: "very selective"},
	2: {floor: 0.65, strong: 0.82, originWeight: 1.00, name: "selective"},
	3: {floor: 0.58, strong: 0.75, originWeight: 0.60, name: "moderately selective"},
}

// ---------------- the score ----------------

type GradeScore struct {
	Pct          float64 `json:"pct"`           // raw GPA as a fraction of its scale
	Adjusted     float64 `json:"adjusted"`      // after the origin/target weighting
	OriginTier   int     `json:"origin_tier"`   // 1–4 (0 for school grades)
	OriginKnown  bool    `json:"origin_known"`  // was the university recognised?
	TargetTier   int     `json:"target_tier"`   // 1–3
	Fraction     float64 `json:"fraction"`      // 0–1, used as the grades component
	Band         string  `json:"band"`          // strong | good | borderline | weak
	TargetStrong float64 `json:"target_strong"` // adjusted level the programme considers strong
}

// ScoreGrade computes the weighted grade score. university is empty for
// school grades (bachelor applicants), where no origin weighting applies.
func ScoreGrade(pct float64, university string, school bool, selectivity int) GradeScore {
	lvl, ok := targetLevels[selectivity]
	if !ok {
		selectivity, lvl = 2, targetLevels[2]
	}
	g := GradeScore{Pct: pct, TargetTier: selectivity, TargetStrong: lvl.strong}

	bonus := 0.0
	if !school {
		g.OriginTier, g.OriginKnown = OriginTier(university)
		bonus = originBonus[g.OriginTier] * lvl.originWeight
	}
	g.Adjusted = math.Min(1, pct*(1+bonus))

	x := (g.Adjusted - lvl.floor) / (lvl.strong - lvl.floor)
	x = math.Max(0, math.Min(1, x))
	g.Fraction = x * x * (3 - 2*x) // smoothstep

	switch {
	case g.Fraction >= 0.85:
		g.Band = "strong"
	case g.Fraction >= 0.55:
		g.Band = "good"
	case g.Fraction >= 0.25:
		g.Band = "borderline"
	default:
		g.Band = "weak"
	}
	return g
}

// Explain turns a score into one plain sentence for the student.
func (g GradeScore) Explain(what string) string {
	pc := int(math.Round(g.Pct * 100))
	adj := int(math.Round(g.Adjusted * 100))
	level := targetLevels[g.TargetTier].name
	strong := int(math.Round(g.TargetStrong * 100))
	if g.OriginTier == 0 { // school grades
		return fmt.Sprintf("Your %s is %d%% of the scale; for a %s programme %d%%+ is strong.", what, pc, level, strong)
	}
	from := originTierName[g.OriginTier]
	if !g.OriginKnown {
		from = "a university we haven't rated yet (counted as neutral)"
	}
	if adj == pc {
		return fmt.Sprintf("Your %s is %d%% from %s; for a %s programme %d%%+ is strong.", what, pc, from, level, strong)
	}
	return fmt.Sprintf("Your %s is %d%% from %s, which counts as about %d%% for a %s programme (%d%%+ is strong).",
		what, pc, from, adj, level, strong)
}
