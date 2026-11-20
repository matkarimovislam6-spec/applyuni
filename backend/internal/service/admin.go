package service

// ApplyUni staff ("platform admins") review documents for every applicant,
// for students who apply on their own and have no agency staff. Admins are
// listed by username in PLATFORM_ADMINS. Every action is written to the
// applicant's audit log under the admin's user id.

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"

	"github.com/matkarimovislam6-spec/applyuni/backend/internal/auth"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/db"
)

// AdminReviewItem is one document waiting for a person, from any organisation.
type AdminReviewItem struct {
	ReviewItem
	Programs []string `json:"programs"` // what the student is applying to
}

// IsPlatformAdmin reports whether the logged-in user is ApplyUni staff.
func (s *Service) IsPlatformAdmin(ctx context.Context, c *auth.Claims) bool {
	if c == nil || len(s.PlatformAdmins) == 0 {
		return false
	}
	var username string
	if err := s.DB.Q().QueryRow(ctx, `SELECT username FROM users WHERE id = ?`, c.UserID).Scan(&username); err != nil {
		return false
	}
	return s.PlatformAdmins[strings.ToLower(username)]
}

// AdminReviewQueue lists documents waiting for review across all applicants,
// oldest first.
func (s *Service) AdminReviewQueue(ctx context.Context, c *auth.Claims) ([]AdminReviewItem, error) {
	if !s.IsPlatformAdmin(ctx, c) {
		return nil, ErrNotFound
	}
	out := []AdminReviewItem{}
	err := s.DB.Tx(ctx, func(tx db.Conn) error {
		rows, err := tx.Query(ctx, `SELECT d.id, d.student_id, d.doc_type, d.filename, d.content_type, d.size_bytes, d.status, d.ai_result, d.verdict, d.pages, d.created_at, s.full_name, s.profile
			FROM documents d JOIN students s ON s.id = d.student_id AND s.org_id = d.org_id
			WHERE d.status = 'needs_review' ORDER BY d.created_at`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			d := &Document{}
			it := AdminReviewItem{ReviewItem: ReviewItem{Document: d}, Programs: []string{}}
			var profile *string
			if err := rows.Scan(&d.ID, &d.StudentID, &d.DocType, &d.Filename, &d.ContentType, &d.SizeBytes, &d.Status, &d.AIResult, &d.Verdict, &d.Pages, &d.CreatedAt, &it.StudentName, &profile); err != nil {
				return err
			}
			if name := passportName(profile); name != "" {
				it.StudentName = name
			}
			out = append(out, it)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for i := range out {
			prows, err := tx.Query(ctx, `SELECT p.name || ' · ' || u.name FROM applications a
				JOIN programs p ON p.id = a.program_id JOIN universities u ON u.id = p.university_id
				WHERE a.student_id = ? ORDER BY a.created_at`, out[i].StudentID)
			if err != nil {
				return err
			}
			for prows.Next() {
				var n string
				if err := prows.Scan(&n); err != nil {
					prows.Close()
					return err
				}
				out[i].Programs = append(out[i].Programs, n)
			}
			prows.Close()
		}
		return nil
	})
	return out, err
}

// AdminDocumentFile returns a document's file for review.
func (s *Service) AdminDocumentFile(ctx context.Context, c *auth.Claims, docID uuid.UUID) (*Document, []byte, error) {
	if !s.IsPlatformAdmin(ctx, c) {
		return nil, nil, ErrNotFound
	}
	orgID, err := s.docOrg(ctx, docID)
	if err != nil {
		return nil, nil, err
	}
	var doc *Document
	var data []byte
	err = s.DB.Tenant(ctx, orgID, func(tx db.Conn) error {
		if doc, data, err = s.loadPlaintext(ctx, tx, orgID, docID); err != nil {
			return err
		}
		return auditIn(ctx, tx, orgID, c, "document.admin_view", docID.String())
	})
	return doc, data, err
}

// AdminReviewDocument accepts or rejects any applicant's document.
func (s *Service) AdminReviewDocument(ctx context.Context, c *auth.Claims, docID uuid.UUID, accept bool, note string) (*Document, error) {
	if !s.IsPlatformAdmin(ctx, c) {
		return nil, ErrNotFound
	}
	orgID, err := s.docOrg(ctx, docID)
	if err != nil {
		return nil, err
	}
	return s.reviewIn(ctx, c, orgID, docID, accept, note)
}

func (s *Service) docOrg(ctx context.Context, docID uuid.UUID) (uuid.UUID, error) {
	var orgID uuid.UUID
	err := s.DB.Tx(ctx, func(tx db.Conn) error {
		return tx.QueryRow(ctx, `SELECT org_id FROM documents WHERE id = ?`, docID).Scan(&orgID)
	})
	if err != nil {
		return uuid.Nil, ErrNotFound
	}
	return orgID, nil
}

// auditIn writes to organisation orgID's audit log (the reviewer may belong
// to another organisation).
func auditIn(ctx context.Context, tx db.Conn, orgID uuid.UUID, c *auth.Claims, action, target string) error {
	_, err := tx.Exec(ctx, `INSERT INTO audit_log(org_id, user_id, action, target, at) VALUES (?, ?, ?, ?, ?)`,
		orgID, c.UserID, action, target, now())
	return err
}

// passportName is "FIRST LAST" from the profile's passport details, if given.
func passportName(profile *string) string {
	if profile == nil {
		return ""
	}
	var p struct {
		First string `json:"legal_first_name"`
		Last  string `json:"legal_last_name"`
	}
	if json.Unmarshal([]byte(*profile), &p) != nil {
		return ""
	}
	return strings.TrimSpace(p.First + " " + p.Last)
}
