package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/matkarimovislam6-spec/applyuni/backend/internal/crypto"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/db"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/mail"
)

// Every student gets a platform address (s-…@MAIL_DOMAIN). We register them
// on university portals with it, so verification codes, "application
// received" notes, requests for documents and decisions all arrive here.
// The mail provider (Mailgun, Postmark, SES…) forwards each message to
// POST /api/inbound/email; ReceiveEmail stores it encrypted, links it to the
// application it is about and moves that application along.

var addrOnly = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)

// InboundEmail is one message as the mail provider hands it over.
type InboundEmail struct {
	To      string `json:"to"`
	From    string `json:"from"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
}

// ReceiveEmail stores and acts on one inbound message.
func (s *Service) ReceiveEmail(ctx context.Context, m InboundEmail) error {
	to := strings.ToLower(addrOnly.FindString(m.To))
	if to == "" || len(m.Text) > 1<<20 {
		return fmt.Errorf("%w: bad message", ErrInvalid)
	}
	// The address alone says which student (and tenant) this is for.
	var orgID, studentID uuid.UUID
	if err := s.DB.Q().QueryRow(ctx, `SELECT org_id, id FROM students WHERE email_alias = ?`, to).Scan(&orgID, &studentID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	kind := mail.Classify(m.Subject, m.Text)
	return s.DB.Tenant(ctx, orgID, func(tx db.Conn) error {
		app, err := s.matchApplication(ctx, tx, orgID, studentID, m)
		if err != nil {
			return err
		}
		id := uuid.New()
		sealed, err := s.Env.Seal(ctx, []byte(m.Text), id[:])
		if err != nil {
			return err
		}
		var appArg any
		if app != nil {
			appArg = app.ID
		}
		if _, err := tx.Exec(ctx, `INSERT INTO inbox_messages(id, org_id, student_id, application_id, to_addr, from_addr, subject, body_ct, nonce, wrapped_dek, key_ref, kind, received_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, orgID, studentID, appArg, to, truncate(m.From, 300), truncate(m.Subject, 500),
			sealed.Ciphertext, sealed.Nonce, sealed.WrappedDEK, sealed.KeyRef, string(kind), now()); err != nil {
			return err
		}
		if app == nil {
			return nil // kept in the student's inbox; not about an application we know
		}
		subject := strings.TrimSpace(m.Subject)
		switch {
		case kind == mail.Verification:
			return nil // used by the portal agent, not news for the student
		case kind.IsDecision():
			if _, err := tx.Exec(ctx, `UPDATE applications SET decision = ?, decision_at = ?, updated_at = ? WHERE org_id = ? AND id = ?`,
				string(kind), now(), now(), orgID, app.ID); err != nil {
				return err
			}
			return event(ctx, tx, orgID, app.ID, "decision_"+string(kind), fmt.Sprintf("%s: %q", app.Program.University, subject))
		case kind == mail.ActionRequired:
			return park(ctx, tx, orgID, app.ID, fmt.Sprintf("%s asked for something: %q. We'll tell you exactly what to send.", app.Program.University, subject))
		case kind == mail.Received:
			return event(ctx, tx, orgID, app.ID, "received_by_university", fmt.Sprintf("%s confirmed it received your application.", app.Program.University))
		default:
			return event(ctx, tx, orgID, app.ID, "email", fmt.Sprintf("Email from %s: %q", app.Program.University, subject))
		}
	})
}

// matchApplication finds the application an email is about: the sender's
// domain must belong to the university, and if the student applies to
// several programmes there, the email must name the programme.
func (s *Service) matchApplication(ctx context.Context, tx db.Conn, orgID, studentID uuid.UUID, m InboundEmail) (*Application, error) {
	apps, err := scanApps(ctx, tx, appSelect+` WHERE org_id = ? AND student_id = ? AND status IN ('approved', 'submitting', 'submitted', 'needs_attention')`, orgID, studentID)
	if err != nil {
		return nil, err
	}
	domain := mail.Domain(m.From)
	var same []*Application
	for _, a := range apps {
		if mail.SameOrganisation(domain, a.Program.Website) {
			same = append(same, a)
		}
	}
	if len(same) == 1 {
		return same[0], nil
	}
	text := strings.ToLower(m.Subject + " " + m.Text)
	var named []*Application
	for _, a := range same {
		if strings.Contains(text, strings.ToLower(a.Program.Name)) {
			named = append(named, a)
		}
	}
	if len(named) == 1 {
		return named[0], nil
	}
	return nil, nil // unclear: never guess which application a decision is for
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// Inbox lets the portal agent read verification codes from the stored mail.
func (s *Service) Inbox() *DBInbox { return &DBInbox{s: s, poll: 2 * time.Second} }

// DBInbox implements portal.Inbox on top of inbox_messages.
type DBInbox struct {
	s    *Service
	poll time.Duration
}

func (in *DBInbox) WaitFor(ctx context.Context, to string, since time.Time, re *regexp.Regexp) (string, error) {
	to = strings.ToLower(to)
	for {
		code, err := in.find(ctx, to, since, re)
		if err != nil || code != "" {
			return code, err
		}
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("no verification email for %s: %w", to, ctx.Err())
		case <-time.After(in.poll):
		}
	}
}

func (in *DBInbox) find(ctx context.Context, to string, since time.Time, re *regexp.Regexp) (string, error) {
	rows, err := in.s.DB.Q().Query(ctx, `SELECT id, subject, body_ct, nonce, wrapped_dek, key_ref FROM inbox_messages
		WHERE to_addr = ? AND received_at >= ? ORDER BY received_at DESC LIMIT 20`, to, since.UTC())
	if err != nil {
		return "", err
	}
	type msg struct {
		id      uuid.UUID
		subject string
		sealed  crypto.Sealed
	}
	var msgs []msg
	for rows.Next() {
		var m msg
		if err := rows.Scan(&m.id, &m.subject, &m.sealed.Ciphertext, &m.sealed.Nonce, &m.sealed.WrappedDEK, &m.sealed.KeyRef); err != nil {
			rows.Close()
			return "", err
		}
		msgs = append(msgs, m)
	}
	rows.Close()
	for _, m := range msgs {
		body, err := in.s.Env.Open(ctx, &m.sealed, m.id[:])
		if err != nil {
			continue
		}
		if g := re.FindStringSubmatch(m.subject + "\n" + string(body)); len(g) > 1 {
			return g[1], nil
		}
	}
	return "", nil
}
