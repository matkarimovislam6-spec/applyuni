package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/matkarimovislam6-spec/applyuni/backend/internal/auth"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/crypto"
)

var allowedTypes = map[string]bool{
	"application/pdf": true, "image/jpeg": true, "image/png": true, "image/webp": true,
}

var docTypeRe = regexp.MustCompile(`^[a-z_]{2,48}$`)

type Document struct {
	ID          uuid.UUID       `json:"id"`
	StudentID   uuid.UUID       `json:"student_id"`
	DocType     string          `json:"doc_type"`
	Filename    string          `json:"filename"`
	ContentType string          `json:"content_type"`
	SizeBytes   int64           `json:"size_bytes"`
	Status      string          `json:"status"`
	AIResult    json.RawMessage `json:"ai_result"`
	CreatedAt   time.Time       `json:"created_at"`
}

const docSelect = `SELECT id, student_id, doc_type, filename, content_type, size_bytes, status, ai_result, created_at FROM documents`

func scanDoc(r pgx.Row) (*Document, error) {
	d := &Document{}
	err := r.Scan(&d.ID, &d.StudentID, &d.DocType, &d.Filename, &d.ContentType, &d.SizeBytes, &d.Status, &d.AIResult, &d.CreatedAt)
	return d, err
}

func (s *Service) UploadDocument(ctx context.Context, c *auth.Claims, studentID uuid.UUID, docType, filename, contentType string, data []byte) (*Document, error) {
	if !docTypeRe.MatchString(docType) {
		return nil, fmt.Errorf("%w: invalid document type", ErrInvalid)
	}
	if !allowedTypes[contentType] {
		return nil, fmt.Errorf("%w: only PDF, JPEG, PNG or WEBP files are accepted", ErrInvalid)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: empty file", ErrInvalid)
	}
	docID := uuid.New()
	key := fmt.Sprintf("%s/%s/%s", c.OrgID, studentID, docID)
	sum := sha256.Sum256(data)

	var doc *Document
	err := s.DB.Tenant(ctx, c.OrgID, func(tx pgx.Tx) error {
		if err := authorizeStudent(ctx, tx, c, studentID); err != nil {
			return err
		}
		sealed, err := s.Env.Seal(ctx, data, docID[:])
		if err != nil {
			return err
		}
		if err := s.Store.Put(ctx, key, sealed.Ciphertext); err != nil {
			return fmt.Errorf("store object: %w", err)
		}
		status := "uploaded"
		if s.AI != nil {
			status = "analyzing"
		}
		doc, err = scanDoc(tx.QueryRow(ctx, `INSERT INTO documents(id,org_id,student_id,doc_type,filename,content_type,size_bytes,sha256,object_key,nonce,wrapped_dek,key_ref,status)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
			RETURNING id, student_id, doc_type, filename, content_type, size_bytes, status, ai_result, created_at`,
			docID, c.OrgID, studentID, docType, filename, contentType, len(data), sum[:], key, sealed.Nonce, sealed.WrappedDEK, sealed.KeyRef, status))
		if err != nil {
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
		return s.refreshApplications(ctx, tx, studentID)
	})
	if err != nil {
		_ = s.Store.Delete(ctx, key) // don't leave orphaned ciphertext
		return nil, err
	}
	return doc, nil
}

func (s *Service) ListDocuments(ctx context.Context, c *auth.Claims, studentID uuid.UUID) ([]*Document, error) {
	out := []*Document{}
	err := s.DB.Tenant(ctx, c.OrgID, func(tx pgx.Tx) error {
		if err := authorizeStudent(ctx, tx, c, studentID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, docSelect+` WHERE student_id=$1 ORDER BY created_at DESC`, studentID)
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

// loadPlaintext decrypts a document. Caller must have authorized access.
func (s *Service) loadPlaintext(ctx context.Context, tx pgx.Tx, docID uuid.UUID) (*Document, []byte, error) {
	var key, ref string
	var nonce, wrapped []byte
	d := &Document{}
	err := tx.QueryRow(ctx, `SELECT id, student_id, doc_type, filename, content_type, size_bytes, status, ai_result, created_at,
		object_key, nonce, wrapped_dek, key_ref FROM documents WHERE id=$1`, docID).
		Scan(&d.ID, &d.StudentID, &d.DocType, &d.Filename, &d.ContentType, &d.SizeBytes, &d.Status, &d.AIResult, &d.CreatedAt,
			&key, &nonce, &wrapped, &ref)
	if errors.Is(err, pgx.ErrNoRows) {
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
	err := s.DB.Tenant(ctx, c.OrgID, func(tx pgx.Tx) error {
		var sid uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT student_id FROM documents WHERE id=$1`, docID).Scan(&sid); err != nil {
			return ErrNotFound
		}
		if err := authorizeStudent(ctx, tx, c, sid); err != nil {
			return err
		}
		var err error
		if doc, data, err = s.loadPlaintext(ctx, tx, docID); err != nil {
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
	err := s.DB.Tenant(ctx, c.OrgID, func(tx pgx.Tx) error {
		status := "rejected"
		if accept {
			status = "accepted"
		}
		review, _ := json.Marshal(map[string]any{"manual_review": true, "accept": accept, "issues": nonEmpty(note)})
		var err error
		doc, err = scanDoc(tx.QueryRow(ctx, `UPDATE documents SET status=$2, ai_result=coalesce(ai_result,'{}'::jsonb) || $3::jsonb WHERE id=$1
			RETURNING id, student_id, doc_type, filename, content_type, size_bytes, status, ai_result, created_at`, docID, status, review))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := audit(ctx, tx, c, "document.review."+status, docID.String()); err != nil {
			return err
		}
		return s.refreshApplications(ctx, tx, doc.StudentID)
	})
	return doc, err
}

func nonEmpty(s string) []string {
	if s == "" {
		return []string{}
	}
	return []string{s}
}

func (s *Service) DeleteDocument(ctx context.Context, c *auth.Claims, docID uuid.UUID) error {
	var key string
	err := s.DB.Tenant(ctx, c.OrgID, func(tx pgx.Tx) error {
		var sid uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT student_id, object_key FROM documents WHERE id=$1`, docID).Scan(&sid, &key); err != nil {
			return ErrNotFound
		}
		if err := authorizeStudent(ctx, tx, c, sid); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM documents WHERE id=$1`, docID); err != nil {
			return err
		}
		if err := audit(ctx, tx, c, "document.delete", docID.String()); err != nil {
			return err
		}
		return s.refreshApplications(ctx, tx, sid)
	})
	if err != nil {
		return err
	}
	return s.Store.Delete(ctx, key)
}
