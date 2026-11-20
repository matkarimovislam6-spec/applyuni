package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/matkarimovislam6-spec/applyuni/backend/internal/auth"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/requirements"
)

type Application struct {
	ID          uuid.UUID    `json:"id"`
	StudentID   uuid.UUID    `json:"student_id"`
	Status      string       `json:"status"`
	StatusNote  *string      `json:"status_note"`
	PortalRef   *string      `json:"portal_ref"`
	SubmittedAt *time.Time   `json:"submitted_at"`
	CreatedAt   time.Time    `json:"created_at"`
	Program     *ProgramInfo `json:"program"`
}

type ApplicationDetail struct {
	*Application
	Checklist *requirements.Checklist `json:"checklist"`
	Approval  *ApprovalInfo           `json:"approval"`
}

type ApprovalInfo struct {
	ApprovedBy string    `json:"approved_by"`
	ApprovedAt time.Time `json:"approved_at"`
	Snapshot   string    `json:"snapshot_sha256"`
}

func (s *Service) CreateApplication(ctx context.Context, c *auth.Claims, studentID, programID uuid.UUID) (*Application, error) {
	var appID uuid.UUID
	err := s.DB.Tenant(ctx, c.OrgID, func(tx pgx.Tx) error {
		if err := authorizeStudent(ctx, tx, c, studentID); err != nil {
			return err
		}
		if _, err := s.getProgram(ctx, tx, programID); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `INSERT INTO applications(org_id,student_id,program_id) VALUES ($1,$2,$3)
			ON CONFLICT (student_id, program_id) DO NOTHING RETURNING id`, c.OrgID, studentID, programID).Scan(&appID)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: already applying to this programme", ErrConflict)
		}
		if err != nil {
			return err
		}
		return s.refreshApplications(ctx, tx, studentID)
	})
	if err != nil {
		return nil, err
	}
	d, err := s.GetApplication(ctx, c, appID)
	if err != nil {
		return nil, err
	}
	return d.Application, nil
}

const appSelect = `SELECT id, student_id, program_id, status, status_note, portal_ref, submitted_at, created_at FROM applications`

func (s *Service) scanApps(ctx context.Context, tx pgx.Tx, q string, args ...any) ([]*Application, error) {
	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	type row struct {
		a   *Application
		pid uuid.UUID
	}
	var list []row
	for rows.Next() {
		a := &Application{}
		var pid uuid.UUID
		if err := rows.Scan(&a.ID, &a.StudentID, &pid, &a.Status, &a.StatusNote, &a.PortalRef, &a.SubmittedAt, &a.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		list = append(list, row{a, pid})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]*Application, 0, len(list))
	for _, r := range list {
		p, err := s.getProgram(ctx, tx, r.pid)
		if err != nil {
			return nil, err
		}
		r.a.Program = p
		out = append(out, r.a)
	}
	return out, nil
}

func (s *Service) ListApplications(ctx context.Context, c *auth.Claims, studentID uuid.UUID) ([]*Application, error) {
	var out []*Application
	err := s.DB.Tenant(ctx, c.OrgID, func(tx pgx.Tx) error {
		if err := authorizeStudent(ctx, tx, c, studentID); err != nil {
			return err
		}
		var err error
		out, err = s.scanApps(ctx, tx, appSelect+` WHERE student_id=$1 ORDER BY created_at`, studentID)
		return err
	})
	return out, err
}

func (s *Service) GetApplication(ctx context.Context, c *auth.Claims, appID uuid.UUID) (*ApplicationDetail, error) {
	d := &ApplicationDetail{}
	err := s.DB.Tenant(ctx, c.OrgID, func(tx pgx.Tx) error {
		apps, err := s.scanApps(ctx, tx, appSelect+` WHERE id=$1`, appID)
		if err != nil {
			return err
		}
		if len(apps) == 0 {
			return ErrNotFound
		}
		if err := authorizeStudent(ctx, tx, c, apps[0].StudentID); err != nil {
			return err
		}
		d.Application = apps[0]
		if d.Checklist, err = s.checklist(ctx, tx, d.Application); err != nil {
			return err
		}
		ap := &ApprovalInfo{}
		var hash []byte
		err = tx.QueryRow(ctx, `SELECT u.full_name, a.approved_at, a.snapshot_sha256 FROM approvals a JOIN users u ON u.id=a.user_id
			WHERE a.application_id=$1 ORDER BY a.approved_at DESC LIMIT 1`, appID).Scan(&ap.ApprovedBy, &ap.ApprovedAt, &hash)
		if err == nil {
			ap.Snapshot = fmt.Sprintf("%x", hash)
			d.Approval = ap
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		return nil
	})
	return d, err
}

func (s *Service) checklist(ctx context.Context, tx pgx.Tx, a *Application) (*requirements.Checklist, error) {
	var profileRaw json.RawMessage
	if err := tx.QueryRow(ctx, `SELECT profile FROM students WHERE id=$1`, a.StudentID).Scan(&profileRaw); err != nil {
		return nil, err
	}
	var prof requirements.Profile
	_ = json.Unmarshal(profileRaw, &prof) // tolerate partial profiles
	rows, err := tx.Query(ctx, `SELECT id::text, doc_type, status FROM documents WHERE student_id=$1`, a.StudentID)
	if err != nil {
		return nil, err
	}
	docs, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (requirements.UploadedDoc, error) {
		var d requirements.UploadedDoc
		err := r.Scan(&d.ID, &d.Type, &d.Status)
		return d, err
	})
	if err != nil {
		return nil, err
	}
	return requirements.Build(a.Program.CountryRules, a.Program.Requirements, prof,
		requirements.Program{Degree: a.Program.Degree, Language: a.Program.Language}, docs)
}

// refreshApplications moves a student's applications between
// collecting_documents and ready_for_review as documents are accepted.
func (s *Service) refreshApplications(ctx context.Context, tx pgx.Tx, studentID uuid.UUID) error {
	apps, err := s.scanApps(ctx, tx, appSelect+` WHERE student_id=$1 AND status IN ('collecting_documents','ready_for_review')`, studentID)
	if err != nil {
		return err
	}
	for _, a := range apps {
		cl, err := s.checklist(ctx, tx, a)
		if err != nil {
			return err
		}
		next := "collecting_documents"
		if cl.Complete {
			next = "ready_for_review"
		}
		if next != a.Status {
			if _, err := tx.Exec(ctx, `UPDATE applications SET status=$2, updated_at=now() WHERE id=$1`, a.ID, next); err != nil {
				return err
			}
		}
	}
	return nil
}

// Approve records the student's explicit consent for exactly this application
// (programme, profile and document hashes) and queues the portal submission.
func (s *Service) Approve(ctx context.Context, c *auth.Claims, appID uuid.UUID, ip, userAgent string) (*ApplicationDetail, error) {
	err := s.DB.Tenant(ctx, c.OrgID, func(tx pgx.Tx) error {
		apps, err := s.scanApps(ctx, tx, appSelect+` WHERE id=$1 FOR UPDATE`, appID)
		if err != nil {
			return err
		}
		if len(apps) == 0 {
			return ErrNotFound
		}
		a := apps[0]
		if err := authorizeStudent(ctx, tx, c, a.StudentID); err != nil {
			return err
		}
		cl, err := s.checklist(ctx, tx, a)
		if err != nil {
			return err
		}
		if a.Status != "ready_for_review" || !cl.Complete {
			return ErrNotReady
		}

		snap, err := s.snapshot(ctx, tx, a, cl)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(snap)
		if _, err := tx.Exec(ctx, `INSERT INTO approvals(org_id,application_id,user_id,snapshot_sha256,snapshot,ip,user_agent)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`, c.OrgID, appID, c.UserID, sum[:], snap, ip, userAgent); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE applications SET status='approved', status_note=NULL, updated_at=now() WHERE id=$1`, appID); err != nil {
			return err
		}
		if err := enqueue(ctx, tx, c.OrgID, "submit_application", map[string]string{"application_id": appID.String()}); err != nil {
			return err
		}
		return audit(ctx, tx, c, "application.approve", appID.String())
	})
	if err != nil {
		return nil, err
	}
	return s.GetApplication(ctx, c, appID)
}

func (s *Service) snapshot(ctx context.Context, tx pgx.Tx, a *Application, cl *requirements.Checklist) ([]byte, error) {
	var profile json.RawMessage
	var name string
	if err := tx.QueryRow(ctx, `SELECT full_name, profile FROM students WHERE id=$1`, a.StudentID).Scan(&name, &profile); err != nil {
		return nil, err
	}
	type docRef struct {
		Type   string `json:"type"`
		ID     string `json:"id"`
		SHA256 string `json:"sha256"`
	}
	docs := []docRef{}
	for _, it := range cl.Items {
		var sum []byte
		if err := tx.QueryRow(ctx, `SELECT sha256 FROM documents WHERE id=$1`, it.DocumentID).Scan(&sum); err != nil {
			return nil, err
		}
		docs = append(docs, docRef{Type: it.Type, ID: it.DocumentID, SHA256: fmt.Sprintf("%x", sum)})
	}
	return json.Marshal(map[string]any{
		"application_id": a.ID, "university": a.Program.University, "program": a.Program.Name,
		"degree": a.Program.Degree, "student": name, "profile": profile, "documents": docs,
	})
}

func enqueue(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, kind string, payload any) error {
	b, _ := json.Marshal(payload)
	_, err := tx.Exec(ctx, `INSERT INTO jobs(org_id,kind,payload) VALUES ($1,$2,$3)`, orgID, kind, b)
	return err
}
