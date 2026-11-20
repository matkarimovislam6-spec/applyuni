package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/matkarimovislam6-spec/applyuni/backend/internal/portal"
)

const maxAttempts = 5

// RunWorker processes background jobs until ctx is cancelled. Several worker
// processes can run at once; SKIP LOCKED hands each job to exactly one.
func (s *Service) RunWorker(ctx context.Context) {
	s.Log.Info("worker started")
	for {
		worked, err := s.runOne(ctx)
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

func (s *Service) runOne(ctx context.Context) (bool, error) {
	var j job
	var payload []byte
	err := s.DB.Pool.QueryRow(ctx, `UPDATE jobs SET status='running', attempts=attempts+1, updated_at=now()
		WHERE id = (SELECT id FROM jobs WHERE status='queued' AND run_after <= now() ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1)
		RETURNING id, org_id, kind, payload, attempts`).Scan(&j.id, &j.orgID, &j.kind, &payload, &j.attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_ = json.Unmarshal(payload, &j.payload)

	switch j.kind {
	case "analyze_document":
		err = s.analyzeDocument(ctx, j)
	case "submit_application":
		err = s.submitApplication(ctx, j)
	default:
		err = fmt.Errorf("unknown job kind %q", j.kind)
	}

	if err == nil {
		_, err = s.DB.Pool.Exec(ctx, `UPDATE jobs SET status='done', last_error=NULL, updated_at=now() WHERE id=$1`, j.id)
		return true, err
	}
	s.Log.Warn("job failed", "id", j.id, "kind", j.kind, "attempt", j.attempts, "err", err)
	if j.attempts >= maxAttempts {
		_, _ = s.DB.Pool.Exec(ctx, `UPDATE jobs SET status='failed', last_error=$2, updated_at=now() WHERE id=$1`, j.id, err.Error())
		s.onJobGaveUp(ctx, j, err)
	} else {
		backoff := time.Duration(1<<j.attempts) * 30 * time.Second
		_, _ = s.DB.Pool.Exec(ctx, `UPDATE jobs SET status='queued', last_error=$2, run_after=now()+$3::interval, updated_at=now() WHERE id=$1`,
			j.id, err.Error(), fmt.Sprintf("%d seconds", int(backoff.Seconds())))
	}
	return true, nil
}

func (s *Service) onJobGaveUp(ctx context.Context, j job, cause error) {
	_ = s.DB.Tenant(ctx, j.orgID, func(tx pgx.Tx) error {
		switch j.kind {
		case "analyze_document":
			_, err := tx.Exec(ctx, `UPDATE documents SET status='uploaded' WHERE id=$1`, j.payload["document_id"])
			return err
		case "submit_application":
			_, err := tx.Exec(ctx, `UPDATE applications SET status='needs_attention', status_note=$2, updated_at=now() WHERE id=$1`,
				j.payload["application_id"], "Automatic submission failed: "+cause.Error())
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
	// Decrypt in a short transaction, call the AI outside it.
	var doc *Document
	var data []byte
	if err := s.DB.Tenant(ctx, j.orgID, func(tx pgx.Tx) error {
		doc, data, err = s.loadPlaintext(ctx, tx, docID)
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
	return s.DB.Tenant(ctx, j.orgID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE documents SET status=$2, ai_result=$3 WHERE id=$1`, docID, status, raw); err != nil {
			return err
		}
		return s.refreshApplications(ctx, tx, doc.StudentID)
	})
}

func (s *Service) submitApplication(ctx context.Context, j job) error {
	appID, err := uuid.Parse(j.payload["application_id"])
	if err != nil {
		return err
	}
	var bundle portal.Bundle
	err = s.DB.Tenant(ctx, j.orgID, func(tx pgx.Tx) error {
		apps, err := s.scanApps(ctx, tx, appSelect+` WHERE id=$1`, appID)
		if err != nil || len(apps) == 0 {
			return fmt.Errorf("load application: %v", err)
		}
		a := apps[0]
		if a.Status != "approved" && a.Status != "submitting" {
			return fmt.Errorf("application status is %s, refusing to submit without approval", a.Status)
		}
		// Submit exactly what was approved: re-check the document hashes.
		var snapRaw []byte
		if err := tx.QueryRow(ctx, `SELECT snapshot FROM approvals WHERE application_id=$1 ORDER BY approved_at DESC LIMIT 1`, appID).Scan(&snapRaw); err != nil {
			return fmt.Errorf("no approval on record: %w", err)
		}
		var snap struct {
			Documents []struct{ Type, ID, SHA256 string } `json:"documents"`
			Profile   map[string]any                      `json:"profile"`
			Student   string                              `json:"student"`
		}
		if err := json.Unmarshal(snapRaw, &snap); err != nil {
			return err
		}
		var alias *string
		_ = tx.QueryRow(ctx, `SELECT email_alias FROM students WHERE id=$1`, a.StudentID).Scan(&alias)
		bundle = portal.Bundle{ApplicationID: a.ID.String(), Portal: a.Program.Portal, University: a.Program.University,
			Program: a.Program.Name, Degree: a.Program.Degree, StudentName: snap.Student, Profile: snap.Profile}
		if alias != nil {
			bundle.StudentEmail = *alias
		}
		for _, d := range snap.Documents {
			id, _ := uuid.Parse(d.ID)
			doc, data, err := s.loadPlaintext(ctx, tx, id)
			if err != nil {
				return fmt.Errorf("document %s changed or missing since approval: %w", d.Type, err)
			}
			if sha256Hex(data) != d.SHA256 {
				return fmt.Errorf("document %s changed since approval", d.Type)
			}
			bundle.Documents = append(bundle.Documents, portal.Document{Type: d.Type, Filename: doc.Filename, ContentType: doc.ContentType, Data: data})
		}
		_, err = tx.Exec(ctx, `UPDATE applications SET status='submitting', updated_at=now() WHERE id=$1`, appID)
		return err
	})
	if err != nil {
		return err
	}

	adapter, err := s.Portals.Get(bundle.Portal)
	if errors.Is(err, portal.ErrNoAdapter) {
		// Not retryable: park the application with a clear note.
		return s.DB.Tenant(ctx, j.orgID, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE applications SET status='needs_attention', status_note=$2, updated_at=now() WHERE id=$1`, appID,
				fmt.Sprintf("Automated submission to %s is not available yet. Your approved package is ready.", bundle.University))
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
	return s.DB.Tenant(ctx, j.orgID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE applications SET status='submitted', portal_ref=$2, status_note=$3, submitted_at=now(), updated_at=now() WHERE id=$1`,
			appID, res.PortalRef, res.Note)
		return err
	})
}
