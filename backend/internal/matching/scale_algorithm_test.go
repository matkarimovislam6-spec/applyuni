package matching

import "testing"

// The example that motivated the algorithm: 80% from a top university beats
// 83% from a less selective one when both apply to a very selective programme.
func TestTopOriginBeatsHigherGPAFromWeakerOrigin(t *testing.T) {
	weaker := ScoreGrade(0.83, "Tashkent University of Information Technologies", false, 1) // tier 3
	top := ScoreGrade(0.80, "Technical University of Munich", false, 1)                     // tier 1
	if top.Adjusted <= weaker.Adjusted || top.Fraction <= weaker.Fraction {
		t.Fatalf("80%% from a top university (adj %.3f, %.2f) should beat 83%% from a national one (adj %.3f, %.2f)",
			top.Adjusted, top.Fraction, weaker.Adjusted, weaker.Fraction)
	}
}

// The origin matters most for very selective programmes and least for moderate ones.
func TestOriginWeightDependsOnTarget(t *testing.T) {
	gap := func(sel int) float64 {
		return ScoreGrade(0.80, "Stanford University", false, sel).Adjusted - ScoreGrade(0.80, "Unknown College", false, sel).Adjusted
	}
	if !(gap(1) > gap(2) && gap(2) > gap(3) && gap(3) > 0) {
		t.Fatalf("origin effect should shrink with selectivity: %.3f %.3f %.3f", gap(1), gap(2), gap(3))
	}
}

// The same grade is worth less at a more selective programme.
func TestSelectivityRaisesTheBar(t *testing.T) {
	f1 := ScoreGrade(0.78, "", false, 1).Fraction
	f2 := ScoreGrade(0.78, "", false, 2).Fraction
	f3 := ScoreGrade(0.78, "", false, 3).Fraction
	if !(f1 < f2 && f2 < f3) {
		t.Fatalf("78%% should score lower at more selective programmes: %.2f %.2f %.2f", f1, f2, f3)
	}
}

func TestOriginTierLookup(t *testing.T) {
	cases := map[string]struct {
		tier  int
		known bool
	}{
		"Westminster International University in Tashkent": {2, true},
		"WIUT":                                  {2, true},
		"Inha University in Tashkent":           {2, true},
		"TUIT":                                  {3, true},
		"Massachusetts Institute of Technology": {1, true},
		"Some Private Institute":                {3, false},
		"":                                      {3, false},
	}
	for name, want := range cases {
		tier, known := OriginTier(name)
		if tier != want.tier || known != want.known {
			t.Errorf("OriginTier(%q) = %d,%v want %d,%v", name, tier, known, want.tier, want.known)
		}
	}
}

func TestSchoolGradesIgnoreOrigin(t *testing.T) {
	a := ScoreGrade(0.90, "Stanford University", true, 1)
	b := ScoreGrade(0.90, "", true, 1)
	if a.Adjusted != b.Adjusted || a.OriginTier != 0 {
		t.Fatalf("school grades must not be weighted by a university: %+v %+v", a, b)
	}
}

func TestBandsAndBounds(t *testing.T) {
	for _, pct := range []float64{0, 0.3, 0.6, 0.7, 0.8, 0.9, 1} {
		for sel := 1; sel <= 3; sel++ {
			g := ScoreGrade(pct, "Harvard University", false, sel)
			if g.Fraction < 0 || g.Fraction > 1 || g.Adjusted > 1 {
				t.Fatalf("out of bounds for pct %.2f sel %d: %+v", pct, sel, g)
			}
		}
	}
	if ScoreGrade(0.95, "", false, 2).Band != "strong" || ScoreGrade(0.55, "", false, 1).Band != "weak" {
		t.Fatal("bands at the extremes are wrong")
	}
	// unknown selectivity falls back to "selective"
	if ScoreGrade(0.8, "", false, 0).TargetTier != 2 {
		t.Fatal("selectivity 0 should default to 2")
	}
}

// End to end: the grades check uses the weighted score and explains it.
func TestGradesCheckUsesWeightedScore(t *testing.T) {
	top := Program{Country: "US", Degree: "master", Language: "en", Meta: Meta{Field: "computer_science", Selectivity: 1}}
	weak := Evaluate(Profile{EducationLevel: "bachelor_done", Major: "computer_science", GPA: "3.32/4", University: "TUIT"}, top, openSoon, now)
	strong := Evaluate(Profile{EducationLevel: "bachelor_done", Major: "computer_science", GPA: "3.2/4", University: "Technical University of Munich"}, top, openSoon, now)
	gw, gs := find(weak, "grades"), find(strong, "grades")
	if gw.Score == nil || gs.Score == nil {
		t.Fatal("grades check should carry the weighted breakdown")
	}
	if gs.Score.Fraction <= gw.Score.Fraction || strong.Score <= weak.Score {
		t.Errorf("3.2/4 from TUM should outscore 3.32/4 from TUIT at a very selective programme (%d vs %d)", strong.Score, weak.Score)
	}
	if gw.Detail == "" {
		t.Error("missing explanation")
	}
}
