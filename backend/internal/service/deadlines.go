package service

import (
	"context"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/matkarimovislam6-spec/applyuni/backend/internal/db"
)

// Deadline is one application round of a programme (non-EU applicants abroad).
type Deadline struct {
	AcademicYear string     `json:"academic_year"`
	Round        string     `json:"round"`
	OpensOn      *time.Time `json:"opens_on"`
	ClosesOn     time.Time  `json:"closes_on"`
	Kind         string     `json:"kind"` // official | previous_year
	FeeEUR       *int       `json:"fee_eur"`
	SourceURL    string     `json:"source_url"`
	VerifiedAt   time.Time  `json:"verified_at"`
	Note         *string    `json:"note"`
	State        string     `json:"state"` // open | upcoming | closed (official rounds); reference (previous_year)
}

// DeadlineStatus summarises the rounds for screening: is it open, and when is the next deadline?
type DeadlineStatus struct {
	State    string    `json:"state"` // open | upcoming | closed | not_published
	Next     *Deadline `json:"next"`  // the round that is open or opens next
	DaysLeft *int      `json:"days_left"`
	LastYear *Deadline `json:"last_year"` // last cycle's deadline, for reference
}

// Deadlines close at the end of the stated day (Italian time).
var rome = func() *time.Location {
	if l, err := time.LoadLocation("Europe/Rome"); err == nil {
		return l
	}
	return time.FixedZone("CET", 3600)
}()

func endOfDay(d time.Time) time.Time {
	y, m, day := d.Date()
	return time.Date(y, m, day, 23, 59, 59, 0, rome)
}

func startOfDay(d time.Time) time.Time {
	y, m, day := d.Date()
	return time.Date(y, m, day, 0, 0, 0, 0, rome)
}

func summarize(ds []Deadline, at time.Time) DeadlineStatus {
	st := DeadlineStatus{State: "not_published"}
	official := false
	for i := range ds {
		d := &ds[i]
		if d.Kind != "official" {
			d.State = "reference"
			if st.LastYear == nil || d.ClosesOn.After(st.LastYear.ClosesOn) {
				st.LastYear = d
			}
			continue
		}
		official = true
		switch {
		case at.After(endOfDay(d.ClosesOn)):
			d.State = "closed"
		case d.OpensOn != nil && at.Before(startOfDay(*d.OpensOn)):
			d.State = "upcoming"
		default:
			d.State = "open"
		}
	}
	if !official {
		return st
	}
	// earliest open round, else earliest upcoming round
	for _, want := range []string{"open", "upcoming"} {
		for i := range ds {
			if ds[i].State == want && (st.Next == nil || ds[i].ClosesOn.Before(st.Next.ClosesOn)) {
				st.Next = &ds[i]
			}
		}
		if st.Next != nil {
			st.State = want
			break
		}
	}
	if st.Next == nil {
		st.State = "closed"
		return st
	}
	days := int(math.Ceil(endOfDay(st.Next.ClosesOn).Sub(at).Hours() / 24))
	st.DaysLeft = &days
	return st
}

// loadDeadlines fills Deadlines and DeadlineStatus for the given programmes.
func loadDeadlines(ctx context.Context, q db.Conn, progs []*ProgramInfo) error {
	if len(progs) == 0 {
		return nil
	}
	byID := map[uuid.UUID]*ProgramInfo{}
	for _, p := range progs {
		byID[p.ID] = p
		p.Deadlines = []Deadline{}
	}
	rows, err := q.Query(ctx, `SELECT program_id, academic_year, round, opens_on, closes_on, kind, fee_eur, source_url, verified_at, note
		FROM program_deadlines ORDER BY closes_on`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var pid uuid.UUID
		var d Deadline
		if err := rows.Scan(&pid, &d.AcademicYear, &d.Round, &d.OpensOn, &d.ClosesOn, &d.Kind, &d.FeeEUR, &d.SourceURL, &d.VerifiedAt, &d.Note); err != nil {
			return err
		}
		if p := byID[pid]; p != nil {
			p.Deadlines = append(p.Deadlines, d)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	at := time.Now()
	for _, p := range progs {
		st := summarize(p.Deadlines, at)
		p.DeadlineStatus = &st
	}
	return nil
}
