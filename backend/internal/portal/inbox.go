package portal

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"regexp"
	"sync"
	"time"
)

// Inbox gives the agent the emails portals send to a student's platform
// alias (verification codes, status updates). The production inbox will be
// backed by the mail service receiving *@inbox.<domain>; MemoryInbox is used
// in tests and development.
type Inbox interface {
	// WaitFor returns the first capture group of re in the first message to
	// `to` received at or after `since`, waiting until ctx is done.
	WaitFor(ctx context.Context, to string, since time.Time, re *regexp.Regexp) (string, error)
}

type message struct {
	to   string
	body string
	at   time.Time
}

type MemoryInbox struct {
	mu   sync.Mutex
	msgs []message
	wake chan struct{}
}

func NewMemoryInbox() *MemoryInbox { return &MemoryInbox{wake: make(chan struct{})} }

// Deliver adds a message (called by the mail receiver, or by tests).
func (m *MemoryInbox) Deliver(to, body string) {
	m.mu.Lock()
	m.msgs = append(m.msgs, message{to: to, body: body, at: time.Now()})
	close(m.wake)
	m.wake = make(chan struct{})
	m.mu.Unlock()
}

func (m *MemoryInbox) WaitFor(ctx context.Context, to string, since time.Time, re *regexp.Regexp) (string, error) {
	for {
		m.mu.Lock()
		for _, msg := range m.msgs {
			if msg.to == to && !msg.at.Before(since) {
				if g := re.FindStringSubmatch(msg.body); len(g) > 1 {
					m.mu.Unlock()
					return g[1], nil
				}
			}
		}
		wake := m.wake
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("no verification email for %s: %w", to, ctx.Err())
		case <-wake:
		}
	}
}

// NewPortalPassword makes a strong password that satisfies the usual portal
// rules (upper, lower, digit, symbol, 20 characters).
func NewPortalPassword() string {
	sets := []string{"ABCDEFGHJKLMNPQRSTUVWXYZ", "abcdefghijkmnopqrstuvwxyz", "23456789", "!@#$%*-_"}
	all := sets[0] + sets[1] + sets[2] + sets[3]
	pick := func(s string) byte {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(s))))
		return s[n.Int64()]
	}
	pw := make([]byte, 0, 20)
	for _, s := range sets {
		pw = append(pw, pick(s))
	}
	for len(pw) < 20 {
		pw = append(pw, pick(all))
	}
	// shuffle so the guaranteed characters aren't always first
	for i := len(pw) - 1; i > 0; i-- {
		j, _ := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		pw[i], pw[j.Int64()] = pw[j.Int64()], pw[i]
	}
	return string(pw)
}
