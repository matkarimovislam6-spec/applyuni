package crypto

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func newTestEnvelope(t *testing.T) *Envelope {
	k := make([]byte, 32)
	rand.Read(k)
	kek, err := NewLocalKEK(base64.StdEncoding.EncodeToString(k))
	if err != nil {
		t.Fatal(err)
	}
	return NewEnvelope(kek)
}

func TestSealOpenRoundTrip(t *testing.T) {
	ctx := context.Background()
	env := newTestEnvelope(t)
	msg := []byte("passport scan bytes")
	s, err := env.Seal(ctx, msg, []byte("doc-1"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(s.Ciphertext, msg) {
		t.Fatal("ciphertext contains plaintext")
	}
	got, err := env.Open(ctx, s, []byte("doc-1"))
	if err != nil || !bytes.Equal(got, msg) {
		t.Fatalf("round trip failed: %v", err)
	}
}

func TestOpenRejectsWrongAADAndTamper(t *testing.T) {
	ctx := context.Background()
	env := newTestEnvelope(t)
	s, _ := env.Seal(ctx, []byte("secret"), []byte("doc-1"))
	if _, err := env.Open(ctx, s, []byte("doc-2")); err == nil {
		t.Fatal("expected failure with wrong AAD (ciphertext moved to another record)")
	}
	s.Ciphertext[0] ^= 1
	if _, err := env.Open(ctx, s, []byte("doc-1")); err == nil {
		t.Fatal("expected failure on tampered ciphertext")
	}
}
