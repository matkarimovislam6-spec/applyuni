package service

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/matkarimovislam6-spec/applyuni/backend/internal/auth"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/db"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/matching"
)

type Match struct {
	Program *ProgramInfo `json:"program"`
	matching.Result
}

func (s *Service) ownProfile(ctx context.Context, c *auth.Claims) (matching.Profile, error) {
	var raw db.JSON
	var p matching.Profile
	err := s.DB.Tenant(ctx, c.OrgID, func(tx db.Conn) error {
		return tx.QueryRow(ctx, `SELECT profile FROM students WHERE org_id = ? AND user_id = ?`, c.OrgID, c.UserID).Scan(&raw)
	})
	if err != nil {
		return p, ErrNotFound // staff accounts have no own student profile
	}
	_ = json.Unmarshal(raw, &p)
	return p, nil
}

func evaluate(p matching.Profile, prog *ProgramInfo, at time.Time) Match {
	return Match{Program: prog, Result: matching.Evaluate(p, matching.Program{
		Degree:   prog.Degree,
		Language: prog.Language,
		Meta:     matching.ParseMeta(json.RawMessage(prog.Requirements)),
	}, deadlineFor(prog), at)}
}

// MatchProgram pre-screens the logged-in student against one programme,
// whatever its degree level (the entry-level check covers mismatches).
func (s *Service) MatchProgram(ctx context.Context, c *auth.Claims, programID uuid.UUID) (*Match, error) {
	p, err := s.ownProfile(ctx, c)
	if err != nil {
		return nil, err
	}
	prog, err := getProgram(ctx, s.DB.Q(), programID)
	if err != nil {
		return nil, err
	}
	m := evaluate(p, prog, time.Now())
	return &m, nil
}

// Matches pre-screens the logged-in student against every programme at the
// degree level they want, best match first.
func (s *Service) Matches(ctx context.Context, c *auth.Claims) ([]Match, error) {
	p, err := s.ownProfile(ctx, c)
	if err != nil {
		return nil, err
	}

	progs, err := s.ListPrograms(ctx, "", p.DegreeTarget)
	if err != nil {
		return nil, err
	}
	at := time.Now()
	out := make([]Match, 0, len(progs))
	for _, prog := range progs {
		out = append(out, evaluate(p, prog, at))
	}
	rank := map[string]int{"strong": 0, "possible": 1, "long_shot": 2, "not_eligible": 3, "closed": 4}
	sort.SliceStable(out, func(i, j int) bool {
		if a, b := rank[out[i].Verdict], rank[out[j].Verdict]; a != b {
			return a < b
		}
		return out[i].Score > out[j].Score
	})
	return out, nil
}

func deadlineFor(p *ProgramInfo) matching.Deadline {
	st := p.DeadlineStatus
	if st == nil {
		return matching.Deadline{State: "not_published"}
	}
	d := matching.Deadline{State: st.State, DaysLeft: st.DaysLeft}
	if st.Next != nil {
		d.Closes = st.Next.ClosesOn.Format("2 Jan 2006")
	}
	return d
}
