package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/matkarimovislam6-spec/applyuni/backend/internal/crypto"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/db"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/portal"
)

// portalLogin returns the student's login for a portal, creating it (with a
// fresh strong password, stored encrypted) the first time. The username is
// the student's platform inbox, so the agent receives the portal's emails.
func (s *Service) portalLogin(ctx context.Context, tx db.Conn, orgID, studentID uuid.UUID, portalID, username string) (string, string, error) {
	var id uuid.UUID
	var user, ref string
	var ct, nonce, wrapped []byte
	err := tx.QueryRow(ctx, `SELECT id, username, secret_ct, nonce, wrapped_dek, key_ref FROM portal_accounts
		WHERE org_id = ? AND student_id = ? AND portal = ?`, orgID, studentID, portalID).Scan(&id, &user, &ct, &nonce, &wrapped, &ref)
	if err == nil {
		pw, err := s.Env.Open(ctx, &crypto.Sealed{Ciphertext: ct, Nonce: nonce, WrappedDEK: wrapped, KeyRef: ref}, id[:])
		if err != nil {
			return "", "", fmt.Errorf("decrypt portal password: %w", err)
		}
		return user, string(pw), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", "", err
	}
	id = uuid.New()
	pw := portal.NewPortalPassword()
	sealed, err := s.Env.Seal(ctx, []byte(pw), id[:])
	if err != nil {
		return "", "", err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO portal_accounts(id, org_id, student_id, portal, username, secret_ct, nonce, wrapped_dek, key_ref, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, orgID, studentID, portalID, username, sealed.Ciphertext, sealed.Nonce, sealed.WrappedDEK, sealed.KeyRef, now()); err != nil {
		return "", "", err
	}
	return username, pw, nil
}

// permanent marks a submission error that retrying can't fix (missing data,
// a CAPTCHA, no approval). The worker parks the application straight away.
type permanent struct{ error }

func (p permanent) Unwrap() error { return p.error }

// explain turns a submission error into a sentence for the student.
func explain(err error) string {
	var missing *portal.MissingError
	var step *portal.StepError
	switch {
	case errors.As(err, &missing):
		return "We need a few more things before we can apply: " + missing.Error() + "."
	case errors.Is(err, portal.ErrNeedsHuman):
		return "The university's site asks you to complete a quick check (like a CAPTCHA) before we can continue."
	case errors.Is(err, portal.ErrNotApproved):
		return "This application needs your approval before it can be submitted."
	case errors.As(err, &step):
		return fmt.Sprintf("The university's site didn't respond as expected at step %q. We're looking into it; nothing was submitted.", step.Step)
	}
	return "Automatic submission failed: " + err.Error()
}

// classify decides whether a submission error is worth retrying.
func classify(err error) error {
	var missing *portal.MissingError
	if errors.As(err, &missing) || errors.Is(err, portal.ErrNeedsHuman) || errors.Is(err, portal.ErrNotApproved) {
		return permanent{err}
	}
	return err // StepError and network problems: retry with backoff
}
