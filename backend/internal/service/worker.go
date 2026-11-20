package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/matkarimovislam6-spec/applyuni/backend/internal/db"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/portal"
)

const maxAttempts = 5

// RunWorker processes background jobs until ctx is cancelled.
func (s *Service) RunWorker(ctx context.Context) {
	s.Log.Info("worker started")
	for {
		worked, err := s.RunOne(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			s.Log.Error("worker", "err", err)
		}
		if !worked {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}
	}
}

type job struct {
	id       int64
	orgID    uuid.UUID
	kind     string
	payload  map[string]string
	attempts int
}

// claimSQL takes the oldest ready job. PostgreSQL uses SKIP LOCKED so several
// workers can run; SQLite has a single writer, so a plain UPDATE is safe.
func claimSQL(d db.Dialect) string {
	lock := ""
	if d == db.Postgres {
		lock = " FOR UPDATE SKIP LOCKED"
	}
	return `UPDATE jobs SET status = 'running', attempts = attempts + 1, updated_at = ?
		WHERE id = (SELECT id FROM jobs WHERE status = 'queued' AND run_after <= ? ORDER BY id LIMIT 1` + lock + `)
		RETURNING id, org_id, kind, payload, attempts`
}

// RunOne processes at most one job. It reports whether a job was found.
func (s *Service) RunOne(ctx context.Context) (bool, error) {
	var j job
	var payload string
	t := now()
	err := s.DB.Q().QueryRow(ctx, claimSQL(s.DB.Dialect), t, t).Scan(&j.id, &j.orgID, &j.kind, &payload, &j.attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_ = json.Unmarshal([]byte(payload), &j.payload)

	switch j.kind {
	case "analyze_document":
		err = s.analyzeDocument(ctx, j)
	case "submit_application":
		err = s.submitApplication(ctx, j)
	default:
		err = fmt.Errorf("unknown job kind %q", j.kind)
	}

	if err == nil {
		_, err = s.DB.Q().Exec(ctx, `UPDATE jobs SET status = 'done', last_error = NULL, updated_at = ? WHERE id = ?`, now(), j.id)
		return true, err
	}
	s.Log.Warn("job failed", "id", j.id, "kind", j.kind, "attempt", j.attempts, "err", err)
	if j.attempts >= maxAttempts {
		_, _ = s.DB.Q().Exec(ctx, `UPDATE jobs SET status = 'failed', last_error = ?, updated_at = ? WHERE id = ?`, err.Error(), now(), j.id)
		s.onJobGaveUp(ctx, j, err)
	} else {
		backoff := time.Duration(1<<j.attempts) * 30 * time.Second
		_, _ = s.DB.Q().Exec(ctx, `UPDATE jobs SET status = 'queued', last_error = ?, run_after = ?, updated_at = ? WHERE id = ?`,
			err.Error(), now().Add(backoff), now(), j.id)
	}
	return true, nil
}

func (s *Service) onJobGaveUp(ctx context.Context, j job, cause error) {
	_ = s.DB.Tenant(ctx, j.orgID, func(tx db.Conn) error {
		switch j.kind {
		case "analyze_document":
			_, err := tx.Exec(ctx, `UPDATE documents SET status = 'uploaded' WHERE org_id = ? AND id = ?`, j.orgID, j.payload["document_id"])
			return err
		case "submit_application":
			_, err := tx.Exec(ctx, `UPDATE applications SET status = 'needs_attention', status_note = ?, updated_at = ? WHERE org_id = ? AND id = ?`,
				"Automatic submission failed: "+cause.Error(), now(), j.orgID, j.payload["application_id"])
			return err
		}
		return nil
	})
}

func (s *Service) analyzeDocument(ctx context.Context, j job) error {
	if s.AI == nil {
		return nil
	}
	docID, err := uuid.Parse(j.payload["document_id"])
	if err != nil {
		return err
	}
	// Decrypt in a short transaction; call the AI outside it.
	var doc *Document
	var data []byte
	if err := s.DB.Tenant(ctx, j.orgID, func(tx db.Conn) error {
		doc, data, err = s.loadPlaintext(ctx, tx, j.orgID, docID)
		return err
	}); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil // deleted meanwhile
		}
		return err
	}
	analysis, raw, err := s.AI.AnalyzeDocument(ctx, doc.DocType, doc.ContentType, data)
	if err != nil {
		return err
	}
	status := "rejected"
	if analysis.Accept && analysis.MatchesExpected {
		status = "accepted"
	}
	return s.DB.Tenant(ctx, j.orgID, func(tx db.Conn) error {
		if _, err := tx.Exec(ctx, `UPDATE documents SET status = ?, ai_result = ? WHERE org_id = ? AND id = ?`, status, string(raw), j.orgID, docID); err != nil {
			return err
		}
		return s.refreshApplications(ctx, tx, j.orgID, doc.StudentID)
	})
}

func (s *Service) submitApplication(ctx context.Context, j job) error {
	appID, err := uuid.Parse(j.payload["application_id"])
	if err != nil {
		return err
	}
	var bundle portal.Bundle
	err = s.DB.Tenant(ctx, j.orgID, func(tx db.Conn) error {
		a, err := getApp(ctx, tx, j.orgID, appID)
		if err != nil {
			return fmt.Errorf("load application: %w", err)
		}
		if a.Status != "approved" && a.Status != "submitting" {
			return fmt.Errorf("application status is %s, refusing to submit without approval", a.Status)
		}
		// Submit exactly what was approved: re-check the document hashes.
		var snapRaw string
		if err := tx.QueryRow(ctx, `SELECT snapshot FROM approvals WHERE org_id = ? AND application_id = ? ORDER BY approved_at DESC LIMIT 1`,
			j.orgID, appID).Scan(&snapRaw); err != nil {
			return fmt.Errorf("no approval on record: %w", err)
		}
		var snap struct {
			Documents []struct{ Type, ID, SHA256 string } `json:"documents"`
			Profile   map[string]any                      `json:"profile"`
			Student   string                              `json:"student"`
		}
		if err := json.Unmarshal([]byte(snapRaw), &snap); err != nil {
			return err
		}
		var alias *string
		_ = tx.QueryRow(ctx, `SELECT email_alias FROM students WHERE org_id = ? AND id = ?`, j.orgID, a.StudentID).Scan(&alias)
		bundle = portal.Bundle{ApplicationID: a.ID.String(), Portal: a.Program.Portal, University: a.Program.University,
			Program: a.Program.Name, Degree: a.Program.Degree, StudentName: snap.Student, Profile: snap.Profile}
		if alias != nil {
			bundle.StudentEmail = *alias
		}
		for _, d := range snap.Documents {
			id, _ := uuid.Parse(d.ID)
			doc, data, err := s.loadPlaintext(ctx, tx, j.orgID, id)
			if err != nil {
				return fmt.Errorf("document %s changed or missing since approval: %w", d.Type, err)
			}
			if sha256Hex(data) != d.SHA256 {
				return fmt.Errorf("document %s changed since approval", d.Type)
			}
			bundle.Documents = append(bundle.Documents, portal.Document{Type: d.Type, Filename: doc.Filename, ContentType: doc.ContentType, Data: data})
		}
		_, err = tx.Exec(ctx, `UPDATE applications SET status = 'submitting', updated_at = ? WHERE org_id = ? AND id = ?`, now(), j.orgID, appID)
		return err
	})
	if err != nil {
		return err
	}

	adapter, err := s.Portals.Get(bundle.Portal)
	if errors.Is(err, portal.ErrNoAdapter) {
		// Not retryable: park the application with a clear note.
		return s.DB.Tenant(ctx, j.orgID, func(tx db.Conn) error {
			_, err := tx.Exec(ctx, `UPDATE applications SET status = 'needs_attention', status_note = ?, updated_at = ? WHERE org_id = ? AND id = ?`,
				fmt.Sprintf("Automated submission to %s is not available yet. Your approved package is ready.", bundle.University), now(), j.orgID, appID)
			return err
		})
	}
	if err != nil {
		return err
	}
	res, err := adapter.Submit(ctx, &bundle)
	if err != nil {
		return err
	}
	return s.DB.Tenant(ctx, j.orgID, func(tx db.Conn) error {
		t := now()
		_, err := tx.Exec(ctx, `UPDATE applications SET status = 'submitted', portal_ref = ?, status_note = ?, submitted_at = ?, updated_at = ?
			WHERE org_id = ? AND id = ?`, res.PortalRef, res.Note, t, t, j.orgID, appID)
		return err
	})
}
