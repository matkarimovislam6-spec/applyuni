package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"

	"github.com/matkarimovislam6-spec/applyuni/backend/internal/auth"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/crypto"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/db"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/intake"
)

var docTypeRe = regexp.MustCompile(`^[a-z_]{2,48}$`)

type Document struct {
	ID          uuid.UUID `json:"id"`
	StudentID   uuid.UUID `json:"student_id"`
	DocType     string    `json:"doc_type"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	Status      string    `json:"status"`
	AIResult    db.JSON   `json:"ai_result"`
	Verdict     db.JSON   `json:"verdict"` // see storedVerdict
	Pages       *int      `json:"pages,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

const docCols = `id, student_id, doc_type, filename, content_type, size_bytes, status, ai_result, verdict, pages, created_at`

func scanDoc(r interface{ Scan(...any) error }) (*Document, error) {
	d := &Document{}
	err := r.Scan(&d.ID, &d.StudentID, &d.DocType, &d.Filename, &d.ContentType, &d.SizeBytes, &d.Status, &d.AIResult, &d.Verdict, &d.Pages, &d.CreatedAt)
	return d, err
}

func getDoc(ctx context.Context, tx db.Conn, orgID, docID uuid.UUID) (*Document, error) {
	d, err := scanDoc(tx.QueryRow(ctx, `SELECT `+docCols+` FROM documents WHERE org_id = ? AND id = ?`, orgID, docID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return d, err
}

// UploadDocument is stage 1 of the cycle: inspect the file (its real type
// comes from its bytes, not the browser), ignore exact re-uploads, encrypt
// and store it, mark older versions of the same document as superseded, and
// queue the AI check. Without AI, the file waits for a person (needs_review).
func (s *Service) UploadDocument(ctx context.Context, c *auth.Claims, studentID uuid.UUID, docType, filename, _ string, data []byte) (*Document, error) {
	if !docTypeRe.MatchString(docType) {
		return nil, fmt.Errorf("%w: invalid document type", ErrInvalid)
	}
	info, err := intake.Inspect(data, filename)
	if err != nil {
		if intake.IsProblem(err) {
			return nil, fmt.Errorf("%w: %s", ErrInvalid, err.Error())
		}
		return nil, err
	}
	status := "needs_review"
	if s.AI != nil {
		status = "analyzing"
	}
	sum := sha256.Sum256(data)
	var doc *Document
	var key string
	err = s.DB.Tenant(ctx, c.OrgID, func(tx db.Conn) error {
		if err := authorizeStudent(ctx, tx, c, studentID); err != nil {
			return err
		}
		// The same file again (double click, retry on a bad connection): keep the first.
		var existing uuid.UUID
		err := tx.QueryRow(ctx, `SELECT id FROM documents WHERE org_id = ? AND student_id = ? AND doc_type = ? AND sha256 = ? AND status <> 'superseded'`,
			c.OrgID, studentID, docType, sum[:]).Scan(&existing)
		if err == nil {
			doc, err = getDoc(ctx, tx, c.OrgID, existing)
			return err
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var docID uuid.UUID
		if docID, key, err = s.storeDocument(ctx, tx, c.OrgID, studentID, docType, info.Filename, info.ContentType, data, status); err != nil {
			return err
		}
		if info.Pages > 0 {
			if _, err := tx.Exec(ctx, `UPDATE documents SET pages = ? WHERE org_id = ? AND id = ?`, info.Pages, c.OrgID, docID); err != nil {
				return err
			}
		}
		// A new version replaces the old one (kept for the audit trail and for
		// any submission already approved with it).
		if _, err := tx.Exec(ctx, `UPDATE documents SET status = 'superseded' WHERE org_id = ? AND student_id = ? AND doc_type = ? AND id <> ? AND status <> 'superseded'`,
			c.OrgID, studentID, docType, docID); err != nil {
			return err
		}
		if s.AI != nil {
			if err := enqueue(ctx, tx, c.OrgID, "analyze_document", map[string]string{"document_id": docID.String()}); err != nil {
				return err
			}
		}
		if err := audit(ctx, tx, c, "document.upload", docID.String()); err != nil {
			return err
		}
		if err := s.crossCheck(ctx, tx, c.OrgID, studentID); err != nil {
			return err
		}
		if err := s.refreshApplications(ctx, tx, c.OrgID, studentID); err != nil {
			return err
		}
		doc, err = getDoc(ctx, tx, c.OrgID, docID)
		return err
	})
	if err != nil {
		if key != "" {
			_ = s.Store.Delete(ctx, key) // don't leave orphaned ciphertext
		}
		return nil, err
	}
	return doc, nil
}

// storeDocument encrypts data with its own key, puts the ciphertext in the
// object store and records it. The document ID is bound as associated data,
// so ciphertext can't be swapped onto another record. Callers authorize first.
func (s *Service) storeDocument(ctx context.Context, tx db.Conn, orgID, studentID uuid.UUID, docType, filename, contentType string, data []byte, status string) (uuid.UUID, string, error) {
	docID := uuid.New()
	key := fmt.Sprintf("%s/%s/%s", orgID, studentID, docID)
	sum := sha256.Sum256(data)
	sealed, err := s.Env.Seal(ctx, data, docID[:])
	if err != nil {
		return docID, "", err
	}
	if err := s.Store.Put(ctx, key, sealed.Ciphertext); err != nil {
		return docID, "", fmt.Errorf("store object: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO documents(id, org_id, student_id, doc_type, filename, content_type, size_bytes, sha256,
		object_key, nonce, wrapped_dek, key_ref, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		docID, orgID, studentID, docType, filename, contentType, len(data), sum[:], key,
		sealed.Nonce, sealed.WrappedDEK, sealed.KeyRef, status, now()); err != nil {
		return docID, key, err
	}
	return docID, key, nil
}

func (s *Service) ListDocuments(ctx context.Context, c *auth.Claims, studentID uuid.UUID) ([]*Document, error) {
	out := []*Document{}
	err := s.DB.Tenant(ctx, c.OrgID, func(tx db.Conn) error {
		if err := authorizeStudent(ctx, tx, c, studentID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT `+docCols+` FROM documents WHERE org_id = ? AND student_id = ? AND status <> 'superseded' ORDER BY created_at DESC`, c.OrgID, studentID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			d, err := scanDoc(rows)
			if err != nil {
				return err
			}
			out = append(out, d)
		}
		return rows.Err()
	})
	return out, err
}

// loadPlaintext reads and decrypts a document of orgID. Callers authorize first.
func (s *Service) loadPlaintext(ctx context.Context, tx db.Conn, orgID, docID uuid.UUID) (*Document, []byte, error) {
	var key, ref string
	var nonce, wrapped []byte
	d := &Document{}
	err := tx.QueryRow(ctx, `SELECT `+docCols+`, object_key, nonce, wrapped_dek, key_ref FROM documents WHERE org_id = ? AND id = ?`, orgID, docID).
		Scan(&d.ID, &d.StudentID, &d.DocType, &d.Filename, &d.ContentType, &d.SizeBytes, &d.Status, &d.AIResult, &d.Verdict, &d.Pages, &d.CreatedAt,
			&key, &nonce, &wrapped, &ref)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	ct, err := s.Store.Get(ctx, key)
	if err != nil {
		return nil, nil, fmt.Errorf("read object: %w", err)
	}
	pt, err := s.Env.Open(ctx, &crypto.Sealed{Ciphertext: ct, Nonce: nonce, WrappedDEK: wrapped, KeyRef: ref}, docID[:])
	if err != nil {
		return nil, nil, fmt.Errorf("decrypt document: %w", err)
	}
	return d, pt, nil
}

func (s *Service) DownloadDocument(ctx context.Context, c *auth.Claims, docID uuid.UUID) (*Document, []byte, error) {
	var doc *Document
	var data []byte
	err := s.DB.Tenant(ctx, c.OrgID, func(tx db.Conn) error {
		d, err := getDoc(ctx, tx, c.OrgID, docID)
		if err != nil {
			return err
		}
		if err := authorizeStudent(ctx, tx, c, d.StudentID); err != nil {
			return err
		}
		if doc, data, err = s.loadPlaintext(ctx, tx, c.OrgID, docID); err != nil {
			return err
		}
		return audit(ctx, tx, c, "document.download", docID.String())
	})
	return doc, data, err
}

// ReviewDocument lets B2B staff accept/reject manually (used when AI is off or unsure).
func (s *Service) ReviewDocument(ctx context.Context, c *auth.Claims, docID uuid.UUID, accept bool, note string) (*Document, error) {
	if !c.IsStaff() {
		return nil, ErrForbidden
	}
	var doc *Document
	err := s.DB.Tenant(ctx, c.OrgID, func(tx db.Conn) error {
		d, err := getDoc(ctx, tx, c.OrgID, docID)
		if err != nil {
			return err
		}
		status := "rejected"
		if accept {
			status = "accepted"
		}
		v := loadVerdict(d.Verdict)
		v.Manual, v.Base, v.Cross = true, status, nil
		if note != "" {
			v.Issues = []string{note}
		} else if accept {
			v.Issues = []string{}
		}
		if _, err := tx.Exec(ctx, `UPDATE documents SET status = ?, verdict = ? WHERE org_id = ? AND id = ?`, status, v.json(), c.OrgID, docID); err != nil {
			return err
		}
		if err := audit(ctx, tx, c, "document.review."+status, docID.String()); err != nil {
			return err
		}
		if err := s.crossCheck(ctx, tx, c.OrgID, d.StudentID); err != nil {
			return err
		}
		if err := s.refreshApplications(ctx, tx, c.OrgID, d.StudentID); err != nil {
			return err
		}
		doc, err = getDoc(ctx, tx, c.OrgID, docID)
		return err
	})
	return doc, err
}

func (s *Service) DeleteDocument(ctx context.Context, c *auth.Claims, docID uuid.UUID) error {
	var key string
	err := s.DB.Tenant(ctx, c.OrgID, func(tx db.Conn) error {
		var sid uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT student_id, object_key FROM documents WHERE org_id = ? AND id = ?`, c.OrgID, docID).Scan(&sid, &key); err != nil {
			return ErrNotFound
		}
		if err := authorizeStudent(ctx, tx, c, sid); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM documents WHERE org_id = ? AND id = ?`, c.OrgID, docID); err != nil {
			return err
		}
		if err := audit(ctx, tx, c, "document.delete", docID.String()); err != nil {
			return err
		}
		if err := s.crossCheck(ctx, tx, c.OrgID, sid); err != nil {
			return err
		}
		return s.refreshApplications(ctx, tx, c.OrgID, sid)
	})
	if err != nil {
		return err
	}
	return s.Store.Delete(ctx, key)
}
