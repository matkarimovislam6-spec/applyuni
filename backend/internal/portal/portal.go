// Package portal holds adapters that submit an approved application to a
// university portal. Each portal (Universitaly, Polimi Online Services,
// Unibo Studenti Online, DreamApply, ...) gets its own adapter, built and
// tested against the live portal. Adapters only run after the student's
// explicit per-application approval has been recorded.
package portal

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type Document struct {
	Type        string
	Filename    string
	ContentType string
	Data        []byte // decrypted only for the duration of the submission
}

type Bundle struct {
	ApplicationID string
	Portal        string
	University    string
	Program       string
	Degree        string
	StudentName   string
	StudentEmail  string // platform alias inbox; the agent reads verification codes here
	Profile       map[string]any
	Documents     []Document
}

type Result struct {
	PortalRef string
	Note      string
}

type Adapter interface {
	Submit(ctx context.Context, b *Bundle) (*Result, error)
}

var ErrNoAdapter = errors.New("no automated adapter for this portal yet")

type Registry struct {
	adapters map[string]Adapter
	fallback Adapter
}

func NewRegistry(dryRun bool) *Registry {
	r := &Registry{adapters: map[string]Adapter{}}
	if dryRun {
		r.fallback = DryRun{}
	}
	return r
}

func (r *Registry) Register(portalID string, a Adapter) { r.adapters[portalID] = a }

func (r *Registry) Get(portalID string) (Adapter, error) {
	if a, ok := r.adapters[portalID]; ok {
		return a, nil
	}
	if r.fallback != nil {
		return r.fallback, nil
	}
	return nil, fmt.Errorf("%w (%s)", ErrNoAdapter, portalID)
}

// DryRun simulates a successful submission so the whole flow can be tested
// end to end in development. Never enable PORTAL_DRY_RUN in production.
type DryRun struct{}

func (DryRun) Submit(_ context.Context, b *Bundle) (*Result, error) {
	return &Result{
		PortalRef: fmt.Sprintf("DRYRUN-%s-%d", b.Portal, time.Now().Unix()),
		Note:      fmt.Sprintf("Dry run: %d documents would be uploaded to %s.", len(b.Documents), b.Portal),
	}, nil
}
