package service

import (
	"context"
	"testing"
	"time"
)

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func ptr[T any](v T) *T { return &v }

func polimiRounds() []Deadline {
	return []Deadline{
		{Round: "1st call", OpensOn: ptr(day("2026-10-01")), ClosesOn: day("2026-12-01"), Kind: "official"},
		{Round: "2nd call", OpensOn: ptr(day("2027-01-12")), ClosesOn: day("2027-02-25"), Kind: "official"},
	}
}

func at(s string) time.Time { // noon in Rome on that day
	t := day(s)
	return time.Date(t.Year(), t.Month(), t.Day(), 12, 0, 0, 0, rome)
}

func TestSummarize(t *testing.T) {
	cases := []struct {
		name, now, state, next string
		days                   int
	}{
		{"first call open", "2026-11-20", "open", "1st call", 12},
		{"closing day still open", "2026-12-01", "open", "1st call", 1},
		{"between calls", "2026-12-15", "upcoming", "2nd call", 73},
		{"second call open", "2027-02-01", "open", "2nd call", 25},
		{"all closed", "2027-03-01", "closed", "", 0},
	}
	for _, c := range cases {
		st := summarize(polimiRounds(), at(c.now))
		if st.State != c.state {
			t.Errorf("%s: state %s, want %s", c.name, st.State, c.state)
			continue
		}
		if c.next == "" {
			if st.Next != nil {
				t.Errorf("%s: unexpected next round %s", c.name, st.Next.Round)
			}
			continue
		}
		if st.Next == nil || st.Next.Round != c.next {
			t.Errorf("%s: next %+v, want %s", c.name, st.Next, c.next)
		}
		if st.DaysLeft == nil || *st.DaysLeft != c.days {
			t.Errorf("%s: days left %v, want %d", c.name, st.DaysLeft, c.days)
		}
	}
}

func TestSummarizeLastYearOnly(t *testing.T) {
	ds := []Deadline{{Round: "Pre-selection", ClosesOn: day("2026-05-15"), Kind: "previous_year"}}
	st := summarize(ds, at("2026-11-20"))
	if st.State != "not_published" || st.LastYear == nil || st.Next != nil || ds[0].State != "reference" {
		t.Fatalf("got %+v", st)
	}
	if st := summarize(nil, at("2026-11-20")); st.State != "not_published" {
		t.Fatalf("no data should be not_published, got %s", st.State)
	}
}

// The seeded catalogue must come back with deadlines on both databases.
func TestCatalogueDeadlines(t *testing.T) {
	s, _ := newTestService(t)
	progs, err := s.ListPrograms(context.Background(), "IT", "")
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]*ProgramInfo{}
	for _, p := range progs {
		found[p.University+" / "+p.Name] = p
		if p.DeadlineStatus == nil {
			t.Fatalf("%s has no deadline status", p.Name)
		}
	}
	cse := found["Politecnico di Milano / Computer Science and Engineering"]
	if cse == nil || len(cse.Deadlines) != 2 || cse.ApplicationFee == nil || *cse.ApplicationFee != 150 || cse.FeeNote == nil {
		t.Fatalf("Polimi CSE data missing: %+v", cse)
	}
	if !cse.Deadlines[0].ClosesOn.Equal(day("2026-12-01")) || cse.Deadlines[0].SourceURL == "" || cse.Deadlines[0].VerifiedAt.IsZero() {
		t.Fatalf("Polimi 1st call wrong: %+v", cse.Deadlines[0])
	}
	sap := found["Sapienza University of Rome / Data Science"]
	if sap == nil || sap.DeadlineStatus.State != "not_published" || sap.DeadlineStatus.LastYear == nil {
		t.Fatalf("Sapienza should be not_published with last year's date: %+v", sap.DeadlineStatus)
	}
	if pd := found["University of Padova / Psychological Science"]; pd == nil || pd.CallNote == nil {
		t.Fatal("Padova call note missing")
	}
	// single-programme path (used by the programme page) also carries deadlines
	one, err := getProgram(context.Background(), s.DB.Q(), cse.ID)
	if err != nil || len(one.Deadlines) != 2 || one.DeadlineStatus == nil {
		t.Fatalf("getProgram: %v %+v", err, one)
	}
}

// Matches pre-screen a real account against the seeded catalogue.
func TestMatchesForStudent(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestService(t)
	sess, err := s.Register(ctx, RegisterInput{Username: "bahar", Password: "supersecret1"})
	if err != nil {
		t.Fatal(err)
	}
	c, _ := s.Tokens.Parse(sess.Token)
	me, _ := s.Me(ctx, c)
	prof := []byte(`{"degree_target":"master","education_level":"bachelor_done","major":"computer_science","gpa":"3.33/4","documents_language":"mixed"}`)
	if _, err := s.UpdateProfile(ctx, c, *me.StudentID, prof); err != nil {
		t.Fatal(err)
	}
	ms, err := s.Matches(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) == 0 {
		t.Fatal("no matches")
	}
	for _, m := range ms {
		if m.Program.Degree != "master" {
			t.Errorf("only master's programmes expected, got %s", m.Program.Name)
		}
		if len(m.Checks) == 0 {
			t.Errorf("%s has no checks", m.Program.Name)
		}
	}
	// Padova's only round has closed, so it must sort last with verdict "closed"
	if last := ms[len(ms)-1]; last.Verdict != "closed" || last.Program.University != "University of Padova" {
		t.Errorf("expected Padova closed last, got %s / %s", last.Program.University, last.Verdict)
	}
}
