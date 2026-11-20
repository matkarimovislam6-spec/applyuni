// Package service contains the business logic shared by the HTTP API and the
// background worker. It works on SQLite and PostgreSQL alike (see internal/db).
//
// Tenant rule: every query on tenant tables (students, applications, documents,
// approvals, portal_accounts, jobs, audit_log) filters by org_id explicitly.
package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/matkarimovislam6-spec/applyuni/backend/internal/auth"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/crypto"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/db"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/portal"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/storage"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/verify"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrForbidden = errors.New("forbidden")
	ErrConflict  = errors.New("conflict")
	ErrInvalid   = errors.New("invalid request")
	ErrSeatLimit = errors.New("seat limit reached for your plan")
	ErrNotReady  = errors.New("application is not ready for approval")
	ErrBadLogin  = errors.New("wrong username or password")
)

// Analyzer reads one document (package ai, backed by Claude). An interface
// so tests can use a predictable reader.
type Analyzer interface {
	AnalyzeDocument(ctx context.Context, expectedType, contentType string, data []byte) (*verify.Extraction, json.RawMessage, error)
}

type Service struct {
	DB         *db.DB
	Env        *crypto.Envelope
	Store      storage.Store
	AI         Analyzer // nil = AI disabled: documents wait for a person
	Portals    *portal.Registry
	Tokens     *auth.Issuer
	MailDomain string
	// InboundSecret: shared secret the mail provider sends with each email.
	InboundSecret string
	Log           *slog.Logger
}

func now() time.Time { return time.Now().UTC() }

// ================= Accounts =================

type RegisterInput struct {
	Username        string `json:"username"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirm_password"` // optional; checked when sent
	Email           string `json:"email"`            // optional
	FullName        string `json:"full_name"`        // optional, defaults to the username
	AccountType     string `json:"account_type"`     // "student" (B2C) or "agency"/"school" (B2B)
	OrgName         string `json:"org_name"`
}

type Session struct {
	Token  string    `json:"-"` // sent as an HttpOnly cookie, never in the body
	UserID uuid.UUID `json:"user_id"`
	OrgID  uuid.UUID `json:"org_id"`
	Role   string    `json:"role"`
}

// Usernames: 3–32 characters, letters, digits, dot, underscore or hyphen,
// starting with a letter or digit. Stored lowercased; matching ignores case.
var usernameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,31}$`)

const minPasswordLen = 8

func normalizeUsername(u string) string { return strings.ToLower(strings.TrimSpace(u)) }

func (s *Service) Register(ctx context.Context, in RegisterInput) (*Session, error) {
	in.Username = normalizeUsername(in.Username)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.FullName = strings.TrimSpace(in.FullName)
	switch {
	case !usernameRe.MatchString(in.Username):
		return nil, fmt.Errorf("%w: username must be 3–32 characters: letters, numbers, dots, dashes or underscores", ErrInvalid)
	case len(in.Password) < minPasswordLen:
		return nil, fmt.Errorf("%w: password must be at least %d characters", ErrInvalid, minPasswordLen)
	case len(in.Password) > 128:
		return nil, fmt.Errorf("%w: password is too long", ErrInvalid)
	case strings.EqualFold(in.Password, in.Username):
		return nil, fmt.Errorf("%w: password can't be the same as the username", ErrInvalid)
	case in.ConfirmPassword != "" && in.ConfirmPassword != in.Password:
		return nil, fmt.Errorf("%w: passwords don't match", ErrInvalid)
	case in.Email != "" && !strings.Contains(in.Email, "@"):
		return nil, fmt.Errorf("%w: invalid email", ErrInvalid)
	}
	if in.FullName == "" {
		in.FullName = in.Username
	}
	kind, role, orgName, seats := "personal", "student", in.FullName, 1
	switch in.AccountType {
	case "student", "":
	case "agency", "school":
		if strings.TrimSpace(in.OrgName) == "" {
			return nil, fmt.Errorf("%w: organization name is required", ErrInvalid)
		}
		kind, role, orgName, seats = in.AccountType, "owner", strings.TrimSpace(in.OrgName), 5 // trial seats until billing exists
	default:
		return nil, fmt.Errorf("%w: unknown account type", ErrInvalid)
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return nil, err
	}

	var emailArg any
	if in.Email != "" {
		emailArg = in.Email
	}
	userID, orgID := uuid.New(), uuid.New()
	err = s.DB.Tx(ctx, func(tx db.Conn) error {
		if _, err := tx.Exec(ctx, `INSERT INTO users(id, username, email, password_hash, full_name, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			userID, in.Username, emailArg, hash, in.FullName, now()); err != nil {
			if db.IsUniqueViolation(err) {
				if strings.Contains(err.Error(), "email") {
					return fmt.Errorf("%w: an account with this email already exists", ErrConflict)
				}
				return fmt.Errorf("%w: this username is taken", ErrConflict)
			}
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO organizations(id, name, kind, seat_limit, created_at) VALUES (?, ?, ?, ?, ?)`,
			orgID, orgName, kind, seats, now()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO memberships(user_id, org_id, role) VALUES (?, ?, ?)`, userID, orgID, role); err != nil {
			return err
		}
		if role == "student" { // B2C: the account holder is the student
			_, err := s.insertStudent(ctx, tx, orgID, &userID, in.FullName, in.Email)
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.session(userID, orgID, role)
}

// dummyHash makes a login for an unknown username take as long as a wrong
// password, so response times don't reveal which usernames exist.
var dummyHash, _ = auth.HashPassword("not-a-real-password")

// Login accepts a username (or the email, if the account has one).
func (s *Service) Login(ctx context.Context, login, password string) (*Session, error) {
	var userID, orgID uuid.UUID
	var hash, role string
	login = strings.ToLower(strings.TrimSpace(login))
	err := s.DB.Q().QueryRow(ctx, `
		SELECT u.id, u.password_hash, m.org_id, m.role FROM users u
		JOIN memberships m ON m.user_id = u.id
		WHERE u.username = ? OR u.email = ? ORDER BY m.role LIMIT 1`, login, login).Scan(&userID, &hash, &orgID, &role)
	if err != nil {
		auth.CheckPassword(dummyHash, password)
		return nil, ErrBadLogin
	}
	if !auth.CheckPassword(hash, password) {
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
	Username  string     `json:"username"`
	FullName  string     `json:"full_name"`
	Email     *string    `json:"email"`
	CreatedAt time.Time  `json:"created_at"`
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
	// Account data is not tenant data: read it outside the tenant transaction.
	if err := s.DB.Q().QueryRow(ctx, `SELECT u.username, u.full_name, u.email, u.created_at, o.name, o.kind, o.plan, o.seat_limit
		FROM users u, organizations o WHERE u.id = ? AND o.id = ?`, c.UserID, c.OrgID).
		Scan(&m.Username, &m.FullName, &m.Email, &m.CreatedAt, &m.OrgName, &m.OrgKind, &m.Plan, &m.SeatLimit); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound // account deleted: the cookie is stale
		}
		return nil, err
	}
	err := s.DB.Tenant(ctx, c.OrgID, func(tx db.Conn) error {
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM students WHERE org_id = ?`, c.OrgID).Scan(&m.SeatsUsed); err != nil {
			return err
		}
		var sid uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM students WHERE org_id = ? AND user_id = ?`, c.OrgID, c.UserID).Scan(&sid); err == nil {
			m.StudentID = &sid
		}
		return nil
	})
	return m, err
}

// ================= Catalogue =================

type ProgramInfo struct {
	ID             uuid.UUID  `json:"id"`
	Name           string     `json:"name"`
	Degree         string     `json:"degree"`
	Language       string     `json:"language"`
	University     string     `json:"university"`
	City           string     `json:"city"`
	Country        string     `json:"country"`
	Website        string     `json:"website"`
	Portal         string     `json:"portal"`
	TuitionEURYear *int       `json:"tuition_eur_year"`
	ApplicationFee *int       `json:"application_fee_eur"`
	Deadline       *time.Time `json:"deadline"`
	SourceURL      *string    `json:"source_url"`
	VerifiedAt     *time.Time `json:"verified_at"`
	Currency       string     `json:"currency"`
	FeeNote        *string    `json:"fee_note"`
	FeeSourceURL   *string    `json:"fee_source_url"`
	CallNote       *string    `json:"call_note"`
	CallSourceURL  *string    `json:"call_source_url"`
	Requirements   db.JSON    `json:"-"`
	CountryRules   db.JSON    `json:"-"`

	Deadlines      []Deadline      `json:"deadlines"`
	DeadlineStatus *DeadlineStatus `json:"deadline_status"`
}

const programSelect = `SELECT p.id, p.name, p.degree, p.language, u.name, coalesce(u.city, ''), c.code, coalesce(u.website, ''), u.portal,
	p.tuition_eur_year, p.application_fee_eur, p.deadline, p.source_url, p.verified_at,
	p.fee_currency, p.fee_note, p.fee_source_url, p.call_note, p.call_source_url, p.requirements, c.rules
	FROM programs p JOIN universities u ON u.id = p.university_id JOIN countries c ON c.code = u.country_code`

func scanProgram(row interface{ Scan(...any) error }) (*ProgramInfo, error) {
	p := &ProgramInfo{}
	err := row.Scan(&p.ID, &p.Name, &p.Degree, &p.Language, &p.University, &p.City, &p.Country, &p.Website, &p.Portal,
		&p.TuitionEURYear, &p.ApplicationFee, &p.Deadline, &p.SourceURL, &p.VerifiedAt,
		&p.Currency, &p.FeeNote, &p.FeeSourceURL, &p.CallNote, &p.CallSourceURL, &p.Requirements, &p.CountryRules)
	return p, err
}

func (s *Service) ListPrograms(ctx context.Context, country, degree string) ([]*ProgramInfo, error) {
	country = strings.ToUpper(country)
	rows, err := s.DB.Q().Query(ctx, programSelect+` WHERE (? = '' OR c.code = ?) AND (? = '' OR p.degree = ?) ORDER BY u.name, p.name`,
		country, country, degree, degree)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	return out, loadDeadlines(ctx, s.DB.Q(), out)
}

func getProgram(ctx context.Context, q db.Conn, id uuid.UUID) (*ProgramInfo, error) {
	p, err := scanProgram(q.QueryRow(ctx, programSelect+` WHERE p.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return p, loadDeadlines(ctx, q, []*ProgramInfo{p})
}

// ================= Students =================

type Student struct {
	ID         uuid.UUID `json:"id"`
	FullName   string    `json:"full_name"`
	Email      *string   `json:"email"`
	EmailAlias *string   `json:"email_alias"`
	Profile    db.JSON   `json:"profile"`
	CreatedAt  time.Time `json:"created_at"`
}

const studentCols = `id, full_name, email, email_alias, profile, created_at`

func scanStudent(row interface{ Scan(...any) error }) (*Student, error) {
	st := &Student{}
	err := row.Scan(&st.ID, &st.FullName, &st.Email, &st.EmailAlias, &st.Profile, &st.CreatedAt)
	return st, err
}

func (s *Service) insertStudent(ctx context.Context, tx db.Conn, orgID uuid.UUID, userID *uuid.UUID, name, email string) (*Student, error) {
	id := uuid.New()
	alias := fmt.Sprintf("s-%s@%s", strings.ReplaceAll(id.String(), "-", "")[:12], s.MailDomain)
	var emailArg any
	if email = strings.ToLower(strings.TrimSpace(email)); email != "" {
		emailArg = email
	}
	if _, err := tx.Exec(ctx, `INSERT INTO students(id, org_id, user_id, full_name, email, email_alias, profile, created_at)
		VALUES (?, ?, ?, ?, ?, ?, '{}', ?)`, id, orgID, userID, name, emailArg, alias, now()); err != nil {
		return nil, err
	}
	return scanStudent(tx.QueryRow(ctx, `SELECT `+studentCols+` FROM students WHERE org_id = ? AND id = ?`, orgID, id))
}

// authorizeStudent checks the student belongs to the caller's org, and that a
// B2C student only touches their own record.
func authorizeStudent(ctx context.Context, tx db.Conn, c *auth.Claims, studentID uuid.UUID) error {
	var owner *uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT user_id FROM students WHERE org_id = ? AND id = ?`, c.OrgID, studentID).Scan(&owner); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
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
	err := s.DB.Tenant(ctx, c.OrgID, func(tx db.Conn) error {
		q := `SELECT ` + studentCols + ` FROM students WHERE org_id = ?`
		args := []any{c.OrgID}
		if !c.IsStaff() {
			q += ` AND user_id = ?`
			args = append(args, c.UserID)
		}
		rows, err := tx.Query(ctx, q+` ORDER BY created_at DESC`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			st, err := scanStudent(rows)
			if err != nil {
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
	var st *Student
	err := s.DB.Tenant(ctx, c.OrgID, func(tx db.Conn) error {
		var seats, used int
		if err := tx.QueryRow(ctx, `SELECT seat_limit FROM organizations WHERE id = ?`, c.OrgID).Scan(&seats); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM students WHERE org_id = ?`, c.OrgID).Scan(&used); err != nil {
			return err
		}
		if used >= seats {
			return ErrSeatLimit
		}
		var err error
		if st, err = s.insertStudent(ctx, tx, c.OrgID, nil, strings.TrimSpace(name), email); err != nil {
			return err
		}
		return audit(ctx, tx, c, "student.create", st.ID.String())
	})
	return st, err
}

func (s *Service) GetStudent(ctx context.Context, c *auth.Claims, id uuid.UUID) (*Student, error) {
	var st *Student
	err := s.DB.Tenant(ctx, c.OrgID, func(tx db.Conn) error {
		if err := authorizeStudent(ctx, tx, c, id); err != nil {
			return err
		}
		var err error
		st, err = scanStudent(tx.QueryRow(ctx, `SELECT `+studentCols+` FROM students WHERE org_id = ? AND id = ?`, c.OrgID, id))
		return err
	})
	return st, err
}

func (s *Service) UpdateProfile(ctx context.Context, c *auth.Claims, id uuid.UUID, profile json.RawMessage) (*Student, error) {
	var probe map[string]any
	if err := json.Unmarshal(profile, &probe); err != nil {
		return nil, fmt.Errorf("%w: profile must be a JSON object", ErrInvalid)
	}
	err := s.DB.Tenant(ctx, c.OrgID, func(tx db.Conn) error {
		if err := authorizeStudent(ctx, tx, c, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE students SET profile = ? WHERE org_id = ? AND id = ?`, string(profile), c.OrgID, id); err != nil {
			return err
		}
		return s.refreshApplications(ctx, tx, c.OrgID, id)
	})
	if err != nil {
		return nil, err
	}
	return s.GetStudent(ctx, c, id)
}

// DeleteStudent erases a student completely: rows (including wrapped keys) and
// the ciphertext objects. Any backup copy of the ciphertext is unreadable
// without the deleted keys (crypto-shredding).
func (s *Service) DeleteStudent(ctx context.Context, c *auth.Claims, id uuid.UUID) error {
	var keys []string
	err := s.DB.Tenant(ctx, c.OrgID, func(tx db.Conn) error {
		if err := authorizeStudent(ctx, tx, c, id); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT object_key FROM documents WHERE org_id = ? AND student_id = ?`, c.OrgID, id)
		if err != nil {
			return err
		}
		for rows.Next() {
			var k string
			if err := rows.Scan(&k); err != nil {
				rows.Close()
				return err
			}
			keys = append(keys, k)
		}
		rows.Close()
		if _, err := tx.Exec(ctx, `DELETE FROM students WHERE org_id = ? AND id = ?`, c.OrgID, id); err != nil {
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

func audit(ctx context.Context, tx db.Conn, c *auth.Claims, action, target string) error {
	_, err := tx.Exec(ctx, `INSERT INTO audit_log(org_id, user_id, action, target, at) VALUES (?, ?, ?, ?, ?)`,
		c.OrgID, c.UserID, action, target, now())
	return err
}

func enqueue(ctx context.Context, tx db.Conn, orgID uuid.UUID, kind string, payload any) error {
	b, _ := json.Marshal(payload)
	t := now()
	_, err := tx.Exec(ctx, `INSERT INTO jobs(org_id, kind, payload, run_after, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		orgID, kind, string(b), t, t, t)
	return err
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
