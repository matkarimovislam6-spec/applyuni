// Package service contains the business logic shared by the HTTP API and the
// background worker.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/matkarimovislam6-spec/applyuni/backend/internal/ai"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/auth"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/crypto"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/db"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/portal"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/storage"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrForbidden = errors.New("forbidden")
	ErrConflict  = errors.New("conflict")
	ErrInvalid   = errors.New("invalid request")
	ErrSeatLimit = errors.New("seat limit reached for your plan")
	ErrNotReady  = errors.New("application is not ready for approval")
	ErrBadLogin  = errors.New("invalid email or password")
)

type Service struct {
	DB         *db.DB
	Env        *crypto.Envelope
	Store      storage.Store
	AI         *ai.Client // nil = AI disabled
	Portals    *portal.Registry
	Tokens     *auth.Issuer
	MailDomain string
	Log        *slog.Logger
}

// ================= Accounts =================

type RegisterInput struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	FullName    string `json:"full_name"`
	AccountType string `json:"account_type"` // "student" (B2C) or "agency"/"school" (B2B)
	OrgName     string `json:"org_name"`
}

type Session struct {
	Token  string    `json:"token"`
	UserID uuid.UUID `json:"user_id"`
	OrgID  uuid.UUID `json:"org_id"`
	Role   string    `json:"role"`
}

func (s *Service) Register(ctx context.Context, in RegisterInput) (*Session, error) {
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))
	if !strings.Contains(in.Email, "@") || len(in.Password) < 10 || strings.TrimSpace(in.FullName) == "" {
		return nil, fmt.Errorf("%w: valid email, full name and a password of at least 10 characters are required", ErrInvalid)
	}
	kind, role, orgName, seats := "personal", "student", in.FullName, 1
	switch in.AccountType {
	case "student", "":
	case "agency", "school":
		if strings.TrimSpace(in.OrgName) == "" {
			return nil, fmt.Errorf("%w: organization name is required", ErrInvalid)
		}
		kind, role, orgName, seats = in.AccountType, "owner", in.OrgName, 5 // trial seats until billing exists
	default:
		return nil, fmt.Errorf("%w: unknown account type", ErrInvalid)
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return nil, err
	}

	var userID, orgID uuid.UUID
	err = pgx.BeginFunc(ctx, s.DB.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO users(email,password_hash,full_name) VALUES ($1,$2,$3) RETURNING id`,
			in.Email, hash, in.FullName).Scan(&userID); err != nil {
			if strings.Contains(err.Error(), "users_email_lower") {
				return fmt.Errorf("%w: an account with this email already exists", ErrConflict)
			}
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO organizations(name,kind,seat_limit) VALUES ($1,$2,$3) RETURNING id`,
			orgName, kind, seats).Scan(&orgID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO memberships(user_id,org_id,role) VALUES ($1,$2,$3)`, userID, orgID, role)
		return err
	})
	if err != nil {
		return nil, err
	}
	if role == "student" {
		// B2C: the account holder is the student.
		if err := s.DB.Tenant(ctx, orgID, func(tx pgx.Tx) error {
			_, err := s.insertStudent(ctx, tx, orgID, &userID, in.FullName, in.Email)
			return err
		}); err != nil {
			return nil, err
		}
	}
	return s.session(userID, orgID, role)
}

func (s *Service) Login(ctx context.Context, email, password string) (*Session, error) {
	var userID, orgID uuid.UUID
	var hash, role string
	err := s.DB.Pool.QueryRow(ctx, `
		SELECT u.id, u.password_hash, m.org_id, m.role FROM users u
		JOIN memberships m ON m.user_id = u.id
		WHERE lower(u.email) = lower($1) ORDER BY m.role LIMIT 1`, strings.TrimSpace(email)).Scan(&userID, &hash, &orgID, &role)
	if err != nil || !auth.CheckPassword(hash, password) {
		return nil, ErrBadLogin
	}
	return s.session(userID, orgID, role)
}

func (s *Service) session(userID, orgID uuid.UUID, role string) (*Session, error) {
	tok, err := s.Tokens.Issue(userID, orgID, role)
	if err != nil {
		return nil, err
	}
	return &Session{Token: tok, UserID: userID, OrgID: orgID, Role: role}, nil
}

type Me struct {
	UserID    uuid.UUID  `json:"user_id"`
	FullName  string     `json:"full_name"`
	Email     string     `json:"email"`
	Role      string     `json:"role"`
	OrgID     uuid.UUID  `json:"org_id"`
	OrgName   string     `json:"org_name"`
	OrgKind   string     `json:"org_kind"`
	Plan      string     `json:"plan"`
	SeatLimit int        `json:"seat_limit"`
	SeatsUsed int        `json:"seats_used"`
	StudentID *uuid.UUID `json:"student_id,omitempty"`
}

func (s *Service) Me(ctx context.Context, c *auth.Claims) (*Me, error) {
	m := &Me{UserID: c.UserID, Role: c.Role, OrgID: c.OrgID}
	if err := s.DB.Pool.QueryRow(ctx, `SELECT u.full_name, u.email, o.name, o.kind, o.plan, o.seat_limit
		FROM users u, organizations o WHERE u.id=$1 AND o.id=$2`, c.UserID, c.OrgID).
		Scan(&m.FullName, &m.Email, &m.OrgName, &m.OrgKind, &m.Plan, &m.SeatLimit); err != nil {
		return nil, err
	}
	err := s.DB.Tenant(ctx, c.OrgID, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM students`).Scan(&m.SeatsUsed); err != nil {
			return err
		}
		var sid uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM students WHERE user_id=$1`, c.UserID).Scan(&sid); err == nil {
			m.StudentID = &sid
		}
		return nil
	})
	return m, err
}

// ================= Catalogue =================

type ProgramInfo struct {
	ID             uuid.UUID       `json:"id"`
	Name           string          `json:"name"`
	Degree         string          `json:"degree"`
	Language       string          `json:"language"`
	University     string          `json:"university"`
	City           string          `json:"city"`
	Country        string          `json:"country"`
	Website        string          `json:"website"`
	Portal         string          `json:"portal"`
	TuitionEURYear *int            `json:"tuition_eur_year"`
	ApplicationFee *int            `json:"application_fee_eur"`
	Deadline       *time.Time      `json:"deadline"`
	SourceURL      *string         `json:"source_url"`
	VerifiedAt     *time.Time      `json:"verified_at"`
	Requirements   json.RawMessage `json:"-"`
	CountryRules   json.RawMessage `json:"-"`
}

const programSelect = `SELECT p.id, p.name, p.degree, p.language, u.name, coalesce(u.city,''), c.code, coalesce(u.website,''), u.portal,
	p.tuition_eur_year, p.application_fee_eur, p.deadline, p.source_url, p.verified_at, p.requirements, c.rules
	FROM programs p JOIN universities u ON u.id = p.university_id JOIN countries c ON c.code = u.country_code`

func scanProgram(row pgx.Row) (*ProgramInfo, error) {
	p := &ProgramInfo{}
	err := row.Scan(&p.ID, &p.Name, &p.Degree, &p.Language, &p.University, &p.City, &p.Country, &p.Website, &p.Portal,
		&p.TuitionEURYear, &p.ApplicationFee, &p.Deadline, &p.SourceURL, &p.VerifiedAt, &p.Requirements, &p.CountryRules)
	return p, err
}

func (s *Service) ListPrograms(ctx context.Context, country, degree string) ([]*ProgramInfo, error) {
	rows, err := s.DB.Pool.Query(ctx, programSelect+` WHERE ($1='' OR c.code=$1) AND ($2='' OR p.degree=$2) ORDER BY u.name, p.name`,
		strings.ToUpper(country), degree)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*ProgramInfo{}
	for rows.Next() {
		p, err := scanProgram(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Service) getProgram(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id uuid.UUID) (*ProgramInfo, error) {
	p, err := scanProgram(q.QueryRow(ctx, programSelect+` WHERE p.id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

// ================= Students =================

type Student struct {
	ID         uuid.UUID       `json:"id"`
	FullName   string          `json:"full_name"`
	Email      *string         `json:"email"`
	EmailAlias *string         `json:"email_alias"`
	Profile    json.RawMessage `json:"profile"`
	CreatedAt  time.Time       `json:"created_at"`
}

func (s *Service) insertStudent(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, userID *uuid.UUID, name, email string) (*Student, error) {
	id := uuid.New()
	alias := fmt.Sprintf("s-%s@%s", strings.ReplaceAll(id.String(), "-", "")[:12], s.MailDomain)
	st := &Student{}
	err := tx.QueryRow(ctx, `INSERT INTO students(id,org_id,user_id,full_name,email,email_alias) VALUES ($1,$2,$3,$4,NULLIF($5,''),$6)
		RETURNING id, full_name, email, email_alias, profile, created_at`, id, orgID, userID, name, email, alias).
		Scan(&st.ID, &st.FullName, &st.Email, &st.EmailAlias, &st.Profile, &st.CreatedAt)
	return st, err
}

// authorizeStudent enforces that B2C students only touch their own record.
// Tenant isolation between orgs is already enforced by RLS.
func authorizeStudent(ctx context.Context, tx pgx.Tx, c *auth.Claims, studentID uuid.UUID) error {
	var owner *uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT user_id FROM students WHERE id=$1`, studentID).Scan(&owner); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if c.IsStaff() {
		return nil
	}
	if owner == nil || *owner != c.UserID {
		return ErrNotFound
	}
	return nil
}

func (s *Service) ListStudents(ctx context.Context, c *auth.Claims) ([]*Student, error) {
	out := []*Student{}
	err := s.DB.Tenant(ctx, c.OrgID, func(tx pgx.Tx) error {
		q := `SELECT id, full_name, email, email_alias, profile, created_at FROM students`
		args := []any{}
		if !c.IsStaff() {
			q += ` WHERE user_id=$1`
			args = append(args, c.UserID)
		}
		rows, err := tx.Query(ctx, q+` ORDER BY created_at DESC`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			st := &Student{}
			if err := rows.Scan(&st.ID, &st.FullName, &st.Email, &st.EmailAlias, &st.Profile, &st.CreatedAt); err != nil {
				return err
			}
			out = append(out, st)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Service) CreateStudent(ctx context.Context, c *auth.Claims, name, email string) (*Student, error) {
	if !c.IsStaff() {
		return nil, ErrForbidden
	}
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("%w: full name is required", ErrInvalid)
	}
	var seats int
	if err := s.DB.Pool.QueryRow(ctx, `SELECT seat_limit FROM organizations WHERE id=$1`, c.OrgID).Scan(&seats); err != nil {
		return nil, err
	}
	var st *Student
	err := s.DB.Tenant(ctx, c.OrgID, func(tx pgx.Tx) error {
		var used int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM students`).Scan(&used); err != nil {
			return err
		}
		if used >= seats {
			return ErrSeatLimit
		}
		var err error
		st, err = s.insertStudent(ctx, tx, c.OrgID, nil, name, email)
		if err == nil {
			err = audit(ctx, tx, c, "student.create", st.ID.String())
		}
		return err
	})
	return st, err
}

func (s *Service) GetStudent(ctx context.Context, c *auth.Claims, id uuid.UUID) (*Student, error) {
	st := &Student{}
	err := s.DB.Tenant(ctx, c.OrgID, func(tx pgx.Tx) error {
		if err := authorizeStudent(ctx, tx, c, id); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT id, full_name, email, email_alias, profile, created_at FROM students WHERE id=$1`, id).
			Scan(&st.ID, &st.FullName, &st.Email, &st.EmailAlias, &st.Profile, &st.CreatedAt)
	})
	return st, err
}

func (s *Service) UpdateProfile(ctx context.Context, c *auth.Claims, id uuid.UUID, profile json.RawMessage) (*Student, error) {
	var probe map[string]any
	if err := json.Unmarshal(profile, &probe); err != nil {
		return nil, fmt.Errorf("%w: profile must be a JSON object", ErrInvalid)
	}
	err := s.DB.Tenant(ctx, c.OrgID, func(tx pgx.Tx) error {
		if err := authorizeStudent(ctx, tx, c, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE students SET profile=$2 WHERE id=$1`, id, profile); err != nil {
			return err
		}
		return s.refreshApplications(ctx, tx, id)
	})
	if err != nil {
		return nil, err
	}
	return s.GetStudent(ctx, c, id)
}

// DeleteStudent erases a student completely. Ciphertext is removed from the
// object store and the wrapped keys are deleted with the rows, so any backup
// copy of the ciphertext is unreadable (crypto-shredding).
func (s *Service) DeleteStudent(ctx context.Context, c *auth.Claims, id uuid.UUID) error {
	var keys []string
	err := s.DB.Tenant(ctx, c.OrgID, func(tx pgx.Tx) error {
		if err := authorizeStudent(ctx, tx, c, id); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT object_key FROM documents WHERE student_id=$1`, id)
		if err != nil {
			return err
		}
		keys, err = pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM students WHERE id=$1`, id); err != nil {
			return err
		}
		return audit(ctx, tx, c, "student.delete", id.String())
	})
	if err != nil {
		return err
	}
	for _, k := range keys {
		if err := s.Store.Delete(ctx, k); err != nil {
			s.Log.Error("delete object", "key", k, "err", err)
		}
	}
	return nil
}

func audit(ctx context.Context, tx pgx.Tx, c *auth.Claims, action, target string) error {
	_, err := tx.Exec(ctx, `INSERT INTO audit_log(org_id,user_id,action,target) VALUES ($1,$2,$3,$4)`, c.OrgID, c.UserID, action, target)
	return err
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
