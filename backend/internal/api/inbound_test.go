package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/matkarimovislam6-spec/applyuni/backend/internal/auth"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/service"
)

// The webhook must refuse anyone without the shared secret, and look absent
// (404) rather than advertise itself. An unknown address is accepted and
// dropped so the provider doesn't retry.
func TestInboundEmailSecret(t *testing.T) {
	const secret = "0123456789abcdef0123456789"
	svc := &service.Service{InboundSecret: secret}
	h := New(svc, auth.NewIssuer("test-secret-test-secret-test-secret-1234"), "http://x", 20, false, slog.New(slog.NewTextHandler(io.Discard, nil)))
	post := func(sec string) int {
		r := httptest.NewRequest("POST", "/api/inbound/email", strings.NewReader(`{"to":"nobody@inbox.test","from":"a@b.it","subject":"x","text":"y"}`))
		if sec != "" {
			r.Header.Set("X-Inbound-Secret", sec)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	if c := post(""); c != http.StatusNotFound {
		t.Errorf("no secret: %d", c)
	}
	if c := post("wrong-secret-wrong-secret-x"); c != http.StatusNotFound {
		t.Errorf("wrong secret: %d", c)
	}
	svc.InboundSecret = ""
	if c := post(""); c != http.StatusNotFound {
		t.Errorf("disabled endpoint: %d", c)
	}
}
