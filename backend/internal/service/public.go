package service

import (
	"context"
	"fmt"
	"net/mail"
	"strings"

	"github.com/google/uuid"

	"github.com/matkarimovislam6-spec/applyuni/backend/internal/requirements"
)

// PublicProgram is a programme with its personalised checklist, for visitors
// without an account. Nothing about the visitor is stored.
type PublicProgram struct {
	*ProgramInfo
	Checklist *requirements.Checklist `json:"checklist"`
}

func (s *Service) PublicChecklist(ctx context.Context, programID uuid.UUID, profile requirements.Profile) (*PublicProgram, error) {
	p, err := s.getProgram(ctx, s.DB.Pool, programID)
	if err != nil {
		return nil, err
	}
	cl, err := requirements.Build(p.CountryRules, p.Requirements, profile,
		requirements.Program{Degree: p.Degree, Language: p.Language}, nil)
	if err != nil {
		return nil, err
	}
	return &PublicProgram{ProgramInfo: p, Checklist: cl}, nil
}

type WaitlistInput struct {
	Email     string     `json:"email"`
	Kind      string     `json:"kind"`
	ProgramID *uuid.UUID `json:"program_id"`
}

func (s *Service) JoinWaitlist(ctx context.Context, in WaitlistInput) error {
	addr, err := mail.ParseAddress(strings.TrimSpace(in.Email))
	if err != nil {
		return fmt.Errorf("%w: please enter a valid email", ErrInvalid)
	}
	if in.Kind != "agency" {
		in.Kind = "student"
	}
	_, err = s.DB.Pool.Exec(ctx, `INSERT INTO waitlist(email, kind, program_id) VALUES ($1,$2,$3)
		ON CONFLICT (lower(email)) DO NOTHING`, addr.Address, in.Kind, in.ProgramID)
	return err
}
