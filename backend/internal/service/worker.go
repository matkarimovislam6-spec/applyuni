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
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/verify"
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
	case "create_portal_account":
		err = s.createPortalAccount(ctx, j)
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
	var perm permanent
	if j.attempts >= maxAttempts || errors.As(err, &perm) {
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
			// the AI couldn't read it: a person will
			_, err := tx.Exec(ctx, `UPDATE documents SET status = 'needs_review' WHERE org_id = ? AND id = ? AND status = 'analyzing'`, j.orgID, j.payload["document_id"])
			return err
		case "create_portal_account", "submit_application":
			appID, err := uuid.Parse(j.payload["application_id"])
			if err != nil {
				return err
			}
			return park(ctx, tx, j.orgID, appID, explain(cause))
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
	// The AI reads; the rules decide (package verify).
	check := verify.Check(doc.DocType, *analysis, now())
	v := storedVerdict{Base: check.Status, Issues: check.Issues, Notes: check.Notes}
	return s.DB.Tenant(ctx, j.orgID, func(tx db.Conn) error {
		res, err := tx.Exec(ctx, `UPDATE documents SET status = ?, ai_result = ?, verdict = ? WHERE org_id = ? AND id = ? AND status = 'analyzing'`,
			v.status(), string(raw), v.json(), j.orgID, docID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil // replaced or reviewed by a person meanwhile
		}
		if err := s.crossCheck(ctx, tx, j.orgID, doc.StudentID); err != nil {
			return err
		}
		return s.refreshApplications(ctx, tx, j.orgID, doc.StudentID)
	})
}

// QueueUncheckedDocuments queues an AI check for every document that was
// uploaded while AI checks were off, so turning them on (adding
// ANTHROPIC_API_KEY) doesn't leave earlier uploads waiting forever.
// Documents a person already reviewed keep their decision.
func (s *Service) QueueUncheckedDocuments(ctx context.Context) (int, error) {
	if s.AI == nil {
		return 0, nil
	}
	n := 0
	err := s.DB.Tx(ctx, func(tx db.Conn) error {
		rows, err := tx.Query(ctx, `SELECT id, org_id FROM documents
			WHERE status = 'needs_review' AND ai_result IS NULL AND verdict IS NULL`)
		if err != nil {
			return err
		}
		type doc struct{ id, org uuid.UUID }
		var docs []doc
		for rows.Next() {
			var d doc
			if err := rows.Scan(&d.id, &d.org); err != nil {
				rows.Close()
				return err
			}
			docs = append(docs, d)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for _, d := range docs {
			payload, _ := json.Marshal(map[string]string{"document_id": d.id.String()})
			var pending int
			if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM jobs WHERE kind = 'analyze_document' AND payload = ? AND status IN ('queued', 'running')`,
				string(payload)).Scan(&pending); err != nil {
				return err
			}
			if pending > 0 {
				continue
			}
			// The analysis only records its result on a document marked as
			// being analysed; this also shows "Checking…" to the applicant.
			if _, err := tx.Exec(ctx, `UPDATE documents SET status = 'analyzing' WHERE org_id = ? AND id = ?`, d.org, d.id); err != nil {
				return err
			}
			if err := enqueue(ctx, tx, d.org, "analyze_document", map[string]string{"document_id": d.id.String()}); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}
