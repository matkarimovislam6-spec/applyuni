package service

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/matkarimovislam6-spec/applyuni/backend/internal/auth"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/db"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/verify"
)

// storedVerdict is the documents.verdict column: the decision on the file
// itself (Base, from verify.Check or a person), plus problems found by
// comparing it with the student's other documents (Cross). The document's
// status is Base, made needs_review while Cross is non-empty.
type storedVerdict struct {
	Base   string   `json:"base"`
	Issues []string `json:"issues"`
	Notes  []string `json:"notes,omitempty"`
	Cross  []string `json:"cross,omitempty"`
	Manual bool     `json:"manual,omitempty"` // decided by a person: comparisons no longer change it
}

func loadVerdict(j db.JSON) storedVerdict {
	var v storedVerdict
	if len(j) > 0 {
		_ = json.Unmarshal(j, &v)
	}
	if v.Issues == nil {
		v.Issues = []string{}
	}
	return v
}

func (v storedVerdict) json() string {
	b, _ := json.Marshal(v)
	return string(b)
}

func (v storedVerdict) status() string {
	if v.Base == verify.Accepted && len(v.Cross) > 0 && !v.Manual {
		return verify.NeedsReview
	}
	return v.Base
}

// crossCheck compares all of a student's current documents with their
// passport and updates statuses. Run whenever a document is added, read by
// the AI, reviewed or deleted.
func (s *Service) crossCheck(ctx context.Context, tx db.Conn, orgID, studentID uuid.UUID) error {
	type row struct {
		id      uuid.UUID
		status  string
		verdict storedVerdict
	}
	rows, err := tx.Query(ctx, `SELECT id, doc_type, status, ai_result, verdict FROM documents
		WHERE org_id = ? AND student_id = ? AND status <> 'superseded' AND ai_result IS NOT NULL`, orgID, studentID)
	if err != nil {
		return err
	}
	var facts []verify.Facts
	var docs []row
	for rows.Next() {
		var r row
		var docType string
		var aiRaw, vRaw db.JSON
		if err := rows.Scan(&r.id, &docType, &r.status, &aiRaw, &vRaw); err != nil {
			rows.Close()
			return err
		}
		var e verify.Extraction
		_ = json.Unmarshal(aiRaw, &e)
		r.verdict = loadVerdict(vRaw)
		if r.verdict.Base == "" {
			continue // not decided yet
		}
		facts = append(facts, verify.Facts{DocumentID: r.id.String(), Type: docType, FullName: e.Fields["full_name"], BirthDate: e.Fields["date_of_birth"]})
		docs = append(docs, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	problems := verify.CrossCheck(facts)
	for _, d := range docs {
		d.verdict.Cross = problems[d.id.String()]
		next := d.verdict.status()
		if _, err := tx.Exec(ctx, `UPDATE documents SET status = ?, verdict = ? WHERE org_id = ? AND id = ?`, next, d.verdict.json(), orgID, d.id); err != nil {
			return err
		}
	}
	return nil
}

// ReviewItem is a document waiting for a person, with what the AI read.
type ReviewItem struct {
	*Document
	StudentName string `json:"student_name"`
}

// ReviewQueue lists the organisation's documents waiting for a person
// (staff only), oldest first so nobody waits longest.
func (s *Service) ReviewQueue(ctx context.Context, c *auth.Claims) ([]ReviewItem, error) {
	if !c.IsStaff() {
		return nil, ErrForbidden
	}
	out := []ReviewItem{}
	err := s.DB.Tenant(ctx, c.OrgID, func(tx db.Conn) error {
		rows, err := tx.Query(ctx, `SELECT d.id, d.student_id, d.doc_type, d.filename, d.content_type, d.size_bytes, d.status, d.ai_result, d.verdict, d.pages, d.created_at, s.full_name
			FROM documents d JOIN students s ON s.id = d.student_id AND s.org_id = d.org_id
			WHERE d.org_id = ? AND d.status = 'needs_review' ORDER BY d.created_at`, c.OrgID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			d := &Document{}
			it := ReviewItem{Document: d}
			if err := rows.Scan(&d.ID, &d.StudentID, &d.DocType, &d.Filename, &d.ContentType, &d.SizeBytes, &d.Status, &d.AIResult, &d.Verdict, &d.Pages, &d.CreatedAt, &it.StudentName); err != nil {
				return err
			}
			out = append(out, it)
		}
		return rows.Err()
	})
	return out, err
}
