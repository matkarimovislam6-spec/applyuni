package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/matkarimovislam6-spec/applyuni/backend/internal/auth"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/db"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/requirements"
)

type Application struct {
	ID          uuid.UUID    `json:"id"`
	OrgID       uuid.UUID    `json:"-"`
	StudentID   uuid.UUID    `json:"student_id"`
	Status      string       `json:"status"`
	StatusNote  *string      `json:"status_note"`
	PortalRef   *string      `json:"portal_ref"`
	SubmittedAt *time.Time   `json:"submitted_at"`
	Decision    *string      `json:"decision"` // the university's answer: admitted | not_admitted | waitlisted
	DecisionAt  *time.Time   `json:"decision_at"`
	CreatedAt   time.Time    `json:"created_at"`
	Program     *ProgramInfo `json:"program"`
}

type ApplicationDetail struct {
	*Application
	Checklist *requirements.Checklist `json:"checklist"`
	Approval  *ApprovalInfo           `json:"approval"`
	Events    []Event                 `json:"events"`
}

type ApprovalInfo struct {
	ApprovedBy string    `json:"approved_by"`
	ApprovedAt time.Time `json:"approved_at"`
	Snapshot   string    `json:"snapshot_sha256"`
}

func (s *Service) CreateApplication(ctx context.Context, c *auth.Claims, studentID, programID uuid.UUID) (*Application, error) {
	appID := uuid.New()
	err := s.DB.Tenant(ctx, c.OrgID, func(tx db.Conn) error {
		if err := authorizeStudent(ctx, tx, c, studentID); err != nil {
			return err
		}
		if _, err := getProgram(ctx, tx, programID); err != nil {
			return err
		}
		t := now()
		if _, err := tx.Exec(ctx, `INSERT INTO applications(id, org_id, student_id, program_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			appID, c.OrgID, studentID, programID, t, t); err != nil {
			if db.IsUniqueViolation(err) {
				return fmt.Errorf("%w: already applying to this programme", ErrConflict)
			}
			return err
		}
		if err := event(ctx, tx, c.OrgID, appID, "created", ""); err != nil {
			return err
		}
		return s.refreshApplications(ctx, tx, c.OrgID, studentID)
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

const appSelect = `SELECT id, org_id, student_id, program_id, status, status_note, portal_ref, submitted_at, decision, decision_at, created_at FROM applications`

// scanApps loads applications and their programmes. The query must filter by org_id.
func scanApps(ctx context.Context, tx db.Conn, q string, args ...any) ([]*Application, error) {
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
		if err := rows.Scan(&a.ID, &a.OrgID, &a.StudentID, &pid, &a.Status, &a.StatusNote, &a.PortalRef, &a.SubmittedAt, &a.Decision, &a.DecisionAt, &a.CreatedAt); err != nil {
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
		p, err := getProgram(ctx, tx, r.pid)
		if err != nil {
			return nil, err
		}
		r.a.Program = p
		out = append(out, r.a)
	}
	return out, nil
}

func getApp(ctx context.Context, tx db.Conn, orgID, appID uuid.UUID) (*Application, error) {
	apps, err := scanApps(ctx, tx, appSelect+` WHERE org_id = ? AND id = ?`, orgID, appID)
	if err != nil {
		return nil, err
	}
	if len(apps) == 0 {
		return nil, ErrNotFound
	}
	return apps[0], nil
}

func (s *Service) ListApplications(ctx context.Context, c *auth.Claims, studentID uuid.UUID) ([]*Application, error) {
	var out []*Application
	err := s.DB.Tenant(ctx, c.OrgID, func(tx db.Conn) error {
		if err := authorizeStudent(ctx, tx, c, studentID); err != nil {
			return err
		}
		var err error
		out, err = scanApps(ctx, tx, appSelect+` WHERE org_id = ? AND student_id = ? ORDER BY created_at`, c.OrgID, studentID)
		return err
	})
	return out, err
}

func (s *Service) GetApplication(ctx context.Context, c *auth.Claims, appID uuid.UUID) (*ApplicationDetail, error) {
	d := &ApplicationDetail{}
	err := s.DB.Tenant(ctx, c.OrgID, func(tx db.Conn) error {
		a, err := getApp(ctx, tx, c.OrgID, appID)
		if err != nil {
			return err
		}
		if err := authorizeStudent(ctx, tx, c, a.StudentID); err != nil {
			return err
		}
		d.Application = a
		if d.Checklist, err = checklist(ctx, tx, a); err != nil {
			return err
		}
		if d.Events, err = loadEvents(ctx, tx, c.OrgID, appID); err != nil {
			return err
		}
		ap := &ApprovalInfo{}
		var hash []byte
		err = tx.QueryRow(ctx, `SELECT u.full_name, a.approved_at, a.snapshot_sha256 FROM approvals a JOIN users u ON u.id = a.user_id
			WHERE a.org_id = ? AND a.application_id = ? ORDER BY a.approved_at DESC LIMIT 1`, c.OrgID, appID).Scan(&ap.ApprovedBy, &ap.ApprovedAt, &hash)
		switch {
		case err == nil:
			ap.Snapshot = fmt.Sprintf("%x", hash)
			d.Approval = ap
		case !errors.Is(err, sql.ErrNoRows):
			return err
		}
		return nil
	})
	return d, err
}

func checklist(ctx context.Context, tx db.Conn, a *Application) (*requirements.Checklist, error) {
	var profileRaw db.JSON
	if err := tx.QueryRow(ctx, `SELECT profile FROM students WHERE org_id = ? AND id = ?`, a.OrgID, a.StudentID).Scan(&profileRaw); err != nil {
		return nil, err
	}
	var prof requirements.Profile
	_ = json.Unmarshal(profileRaw, &prof) // tolerate partial profiles
	rows, err := tx.Query(ctx, `SELECT id, doc_type, status FROM documents WHERE org_id = ? AND student_id = ?`, a.OrgID, a.StudentID)
	if err != nil {
		return nil, err
	}
	var docs []requirements.UploadedDoc
	for rows.Next() {
		var d requirements.UploadedDoc
		if err := rows.Scan(&d.ID, &d.Type, &d.Status); err != nil {
			rows.Close()
			return nil, err
		}
		docs = append(docs, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return requirements.Build(json.RawMessage(a.Program.CountryRules), json.RawMessage(a.Program.Requirements), prof,
		requirements.Program{Degree: a.Program.Degree, Language: a.Program.Language}, docs)
}

// refreshApplications moves a student's applications between
// collecting_documents and ready_for_review as documents are accepted.
func (s *Service) refreshApplications(ctx context.Context, tx db.Conn, orgID, studentID uuid.UUID) error {
	apps, err := scanApps(ctx, tx, appSelect+` WHERE org_id = ? AND student_id = ? AND status IN ('collecting_documents', 'ready_for_review')`, orgID, studentID)
	if err != nil {
		return err
	}
	for _, a := range apps {
		cl, err := checklist(ctx, tx, a)
		if err != nil {
			return err
		}
		next := "collecting_documents"
		if cl.Complete {
			next = "ready_for_review"
		}
		if next != a.Status {
			if _, err := tx.Exec(ctx, `UPDATE applications SET status = ?, updated_at = ? WHERE org_id = ? AND id = ?`, next, now(), orgID, a.ID); err != nil {
				return err
			}
			kind := "documents_complete"
			if next == "collecting_documents" {
				kind = "documents_incomplete"
			}
			if err := event(ctx, tx, orgID, a.ID, kind, ""); err != nil {
				return err
			}
		}
	}
	return nil
}

// Approve records the student's explicit consent for exactly this application
// (programme, profile and document hashes) and queues the portal submission.
func (s *Service) Approve(ctx context.Context, c *auth.Claims, appID uuid.UUID, ip, userAgent string) (*ApplicationDetail, error) {
	err := s.DB.Tenant(ctx, c.OrgID, func(tx db.Conn) error {
		a, err := getApp(ctx, tx, c.OrgID, appID)
		if err != nil {
			return err
		}
		if err := authorizeStudent(ctx, tx, c, a.StudentID); err != nil {
			return err
		}
		cl, err := checklist(ctx, tx, a)
		if err != nil {
			return err
		}
		if a.Status != "ready_for_review" || !cl.Complete {
			return ErrNotReady
		}
		snap, err := snapshot(ctx, tx, a, cl)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(snap)
		if _, err := tx.Exec(ctx, `INSERT INTO approvals(id, org_id, application_id, user_id, snapshot_sha256, snapshot, ip, user_agent, approved_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, uuid.New(), c.OrgID, appID, c.UserID, sum[:], string(snap), ip, userAgent, now()); err != nil {
			return err
		}
		// Guarded update: only one approval can move it out of ready_for_review.
		res, err := tx.Exec(ctx, `UPDATE applications SET status = 'approved', status_note = NULL, updated_at = ?
			WHERE org_id = ? AND id = ? AND status = 'ready_for_review'`, now(), c.OrgID, appID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return ErrNotReady
		}
		// Account first (once per portal), then submission.
		if err := enqueue(ctx, tx, c.OrgID, "create_portal_account", map[string]string{"application_id": appID.String()}); err != nil {
			return err
		}
		if err := event(ctx, tx, c.OrgID, appID, "approved", fmt.Sprintf("Approved by you. Package fingerprint %x.", sum[:6])); err != nil {
			return err
		}
		return audit(ctx, tx, c, "application.approve", appID.String())
	})
	if err != nil {
		return nil, err
	}
	return s.GetApplication(ctx, c, appID)
}

func snapshot(ctx context.Context, tx db.Conn, a *Application, cl *requirements.Checklist) ([]byte, error) {
	var profile db.JSON
	var name string
	if err := tx.QueryRow(ctx, `SELECT full_name, profile FROM students WHERE org_id = ? AND id = ?`, a.OrgID, a.StudentID).Scan(&name, &profile); err != nil {
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
		if err := tx.QueryRow(ctx, `SELECT sha256 FROM documents WHERE org_id = ? AND id = ?`, a.OrgID, it.DocumentID).Scan(&sum); err != nil {
			return nil, err
		}
		docs = append(docs, docRef{Type: it.Type, ID: it.DocumentID, SHA256: fmt.Sprintf("%x", sum)})
	}
	return json.Marshal(map[string]any{
		"application_id": a.ID, "university": a.Program.University, "program": a.Program.Name,
		"degree": a.Program.Degree, "student": name, "profile": json.RawMessage(orEmptyObject(profile)), "documents": docs,
	})
}

func orEmptyObject(j db.JSON) db.JSON {
	if len(j) == 0 {
		return db.JSON("{}")
	}
	return j
}
