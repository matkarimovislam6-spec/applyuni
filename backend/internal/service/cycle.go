package service

// The application cycle, end to end:
//
//   collecting_documents ─(every document accepted)→ ready_for_review
//   ready_for_review ─(student approves: snapshot + hashes)→ approved
//   approved ─(job create_portal_account: register + confirm email)→ approved
//   approved ─(job submit_application: re-check hashes, deadline, fill, submit)→ submitted
//   submitted ─(university email: decision)→ decision = admitted | not_admitted | waitlisted
//   any ─(something only a person can fix)→ needs_attention (with a note saying what)
//
// Every transition writes an application_events row, so the student (and the
// agency) see a timeline of exactly what happened and when.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/matkarimovislam6-spec/applyuni/backend/internal/db"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/portal"
)

// Event is one line of an application's timeline.
type Event struct {
	Kind string    `json:"kind"`
	Note *string   `json:"note"`
	At   time.Time `json:"at"`
}

func event(ctx context.Context, tx db.Conn, orgID, appID uuid.UUID, kind, note string) error {
	var n any
	if note != "" {
		n = note
	}
	_, err := tx.Exec(ctx, `INSERT INTO application_events(org_id, application_id, kind, note, at) VALUES (?, ?, ?, ?, ?)`, orgID, appID, kind, n, now())
	return err
}

func loadEvents(ctx context.Context, tx db.Conn, orgID, appID uuid.UUID) ([]Event, error) {
	rows, err := tx.Query(ctx, `SELECT kind, note, at FROM application_events WHERE org_id = ? AND application_id = ? ORDER BY id`, orgID, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.Kind, &e.Note, &e.At); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// park stops automatic work on an application until a person acts.
func park(ctx context.Context, tx db.Conn, orgID, appID uuid.UUID, note string) error {
	if _, err := tx.Exec(ctx, `UPDATE applications SET status = 'needs_attention', status_note = ?, updated_at = ? WHERE org_id = ? AND id = ?`,
		note, now(), orgID, appID); err != nil {
		return err
	}
	return event(ctx, tx, orgID, appID, "needs_attention", note)
}

// buildBundle assembles exactly what the student approved: the snapshot's
// profile and documents (each re-checked against its approved hash), the
// platform email and the portal login. Only then is Approved set.
func (s *Service) buildBundle(ctx context.Context, tx db.Conn, orgID uuid.UUID, a *Application, withDocuments bool) (*portal.Bundle, error) {
	var snapRaw string
	if err := tx.QueryRow(ctx, `SELECT snapshot FROM approvals WHERE org_id = ? AND application_id = ? ORDER BY approved_at DESC LIMIT 1`,
		orgID, a.ID).Scan(&snapRaw); err != nil {
		return nil, permanent{fmt.Errorf("no approval on record: %w", err)}
	}
	var snap struct {
		Documents []struct{ Type, ID, SHA256 string } `json:"documents"`
		Profile   map[string]any                      `json:"profile"`
		Student   string                              `json:"student"`
	}
	if err := json.Unmarshal([]byte(snapRaw), &snap); err != nil {
		return nil, err
	}
	var alias *string
	_ = tx.QueryRow(ctx, `SELECT email_alias FROM students WHERE org_id = ? AND id = ?`, orgID, a.StudentID).Scan(&alias)
	b := &portal.Bundle{ApplicationID: a.ID.String(), Portal: a.Program.Portal, University: a.Program.University,
		Program: a.Program.Name, Degree: a.Program.Degree, StudentName: snap.Student, Profile: snap.Profile}
	if alias != nil {
		b.StudentEmail = *alias
	}
	if withDocuments {
		for _, d := range snap.Documents {
			id, _ := uuid.Parse(d.ID)
			doc, data, err := s.loadPlaintext(ctx, tx, orgID, id)
			if err != nil {
				return nil, permanent{fmt.Errorf("document %s changed or missing since approval: %w", d.Type, err)}
			}
			if sha256Hex(data) != d.SHA256 {
				return nil, permanent{fmt.Errorf("document %s changed since approval", d.Type)}
			}
			b.Documents = append(b.Documents, portal.Document{Type: d.Type, Filename: doc.Filename, ContentType: doc.ContentType, Data: data})
		}
	}
	user, pw, err := s.portalLogin(ctx, tx, orgID, a.StudentID, a.Program.Portal, b.StudentEmail)
	if err != nil {
		return nil, err
	}
	b.Fields = portal.BuildFields(snap.Student, b.StudentEmail, user, pw, snap.Profile, a.Program.Name, a.Program.Degree)
	b.Approved = true
	return b, nil
}

// createPortalAccount is stage 3: register the student on the university's
// portal once, with their platform email (so we receive the confirmation
// code) and a generated password stored encrypted. Then queue submission.
func (s *Service) createPortalAccount(ctx context.Context, j job) error {
	appID, err := uuid.Parse(j.payload["application_id"])
	if err != nil {
		return err
	}
	var b *portal.Bundle
	var a *Application
	done := false
	err = s.DB.Tenant(ctx, j.orgID, func(tx db.Conn) error {
		if a, err = getApp(ctx, tx, j.orgID, appID); err != nil {
			return err
		}
		if a.Status != "approved" {
			done = true // withdrawn or already moving on
			return nil
		}
		if b, err = s.buildBundle(ctx, tx, j.orgID, a, false); err != nil {
			return err
		}
		var status string
		_ = tx.QueryRow(ctx, `SELECT status FROM portal_accounts WHERE org_id = ? AND student_id = ? AND portal = ?`, j.orgID, a.StudentID, a.Program.Portal).Scan(&status)
		if status == "active" {
			done = true // one account per portal, reused for every programme there
			return enqueue(ctx, tx, j.orgID, "submit_application", map[string]string{"application_id": appID.String()})
		}
		return nil
	})
	if err != nil || done {
		return err
	}

	adapter, err := s.Portals.Get(b.Portal)
	creator, ok := adapter.(portal.AccountCreator)
	if errors.Is(err, portal.ErrNoAdapter) || (err == nil && !ok) {
		// Submission decides what to do (it parks with a clear note).
		return s.DB.Tenant(ctx, j.orgID, func(tx db.Conn) error {
			return enqueue(ctx, tx, j.orgID, "submit_application", map[string]string{"application_id": appID.String()})
		})
	}
	if err != nil {
		return err
	}
	res, runErr := creator.CreateAccount(ctx, b)
	return s.DB.Tenant(ctx, j.orgID, func(tx db.Conn) error {
		if runErr != nil {
			if _, err := tx.Exec(ctx, `UPDATE portal_accounts SET status = 'failed', last_error = ? WHERE org_id = ? AND student_id = ? AND portal = ?`,
				explain(runErr), j.orgID, a.StudentID, b.Portal); err != nil {
				return err
			}
			return classify(runErr)
		}
		if _, err := tx.Exec(ctx, `UPDATE portal_accounts SET status = 'active', verified_at = ?, last_error = NULL WHERE org_id = ? AND student_id = ? AND portal = ?`,
			now(), j.orgID, a.StudentID, b.Portal); err != nil {
			return err
		}
		note := fmt.Sprintf("Account created on the %s portal with your platform email %s.", a.Program.University, b.StudentEmail)
		if res != nil && res.Note != "" {
			note += " " + res.Note
		}
		if err := event(ctx, tx, j.orgID, appID, "account_created", note); err != nil {
			return err
		}
		return enqueue(ctx, tx, j.orgID, "submit_application", map[string]string{"application_id": appID.String()})
	})
}

// submitApplication is stage 4: submit exactly what was approved.
func (s *Service) submitApplication(ctx context.Context, j job) error {
	appID, err := uuid.Parse(j.payload["application_id"])
	if err != nil {
		return err
	}
	var b *portal.Bundle
	var a *Application
	stop := false
	err = s.DB.Tenant(ctx, j.orgID, func(tx db.Conn) error {
		if a, err = getApp(ctx, tx, j.orgID, appID); err != nil {
			return fmt.Errorf("load application: %w", err)
		}
		if a.Status != "approved" && a.Status != "submitting" {
			return permanent{fmt.Errorf("application status is %s, refusing to submit without approval", a.Status)}
		}
		// Never send an application the university will throw away.
		if ds := a.Program.DeadlineStatus; ds != nil && ds.State == "closed" {
			stop = true
			return park(ctx, tx, j.orgID, appID, "The application deadline for this programme has passed, so we did not submit. Your approved package is kept for the next round.")
		}
		if b, err = s.buildBundle(ctx, tx, j.orgID, a, true); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE applications SET status = 'submitting', updated_at = ? WHERE org_id = ? AND id = ?`, now(), j.orgID, appID); err != nil {
			return err
		}
		return event(ctx, tx, j.orgID, appID, "submitting", "")
	})
	if err != nil || stop {
		return err
	}

	adapter, err := s.Portals.Get(b.Portal)
	if errors.Is(err, portal.ErrNoAdapter) {
		return s.DB.Tenant(ctx, j.orgID, func(tx db.Conn) error {
			return park(ctx, tx, j.orgID, appID, fmt.Sprintf("Automated submission to %s is not available yet. Your approved package is ready.", b.University))
		})
	}
	if err != nil {
		return err
	}
	res, err := adapter.Submit(ctx, b)
	if err != nil {
		return classify(err)
	}
	return s.DB.Tenant(ctx, j.orgID, func(tx db.Conn) error {
		// Keep the portal's confirmation page as an encrypted receipt.
		if len(res.Receipt) > 0 {
			if _, _, err := s.storeDocument(ctx, tx, j.orgID, a.StudentID, "submission_receipt",
				fmt.Sprintf("receipt-%s.png", b.Portal), "image/png", res.Receipt, "accepted"); err != nil {
				return err
			}
		}
		t := now()
		if !res.Submitted { // e.g. a flow running in dry-run mode
			return park(ctx, tx, j.orgID, appID, res.Note)
		}
		if _, err := tx.Exec(ctx, `UPDATE applications SET status = 'submitted', portal_ref = ?, status_note = ?, submitted_at = ?, updated_at = ?
			WHERE org_id = ? AND id = ?`, res.PortalRef, res.Note, t, t, j.orgID, appID); err != nil {
			return err
		}
		note := fmt.Sprintf("Submitted to %s.", b.University)
		if res.PortalRef != "" {
			note += " Confirmation number: " + res.PortalRef + "."
		}
		return event(ctx, tx, j.orgID, appID, "submitted", note)
	})
}
