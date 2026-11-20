// Package crypto implements envelope encryption for student files and secrets.
//
// Every file gets a fresh random 256-bit data key (DEK). The file is encrypted
// with AES-256-GCM under that DEK, and the DEK itself is encrypted ("wrapped")
// by a key-encryption key (KEK) that never leaves the key manager (Vault/OpenBao
// transit in production). The database stores only the wrapped DEK; the object
// store holds only ciphertext. Stealing either one alone reveals nothing.
package crypto

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type KeyWrapper interface {
	Wrap(ctx context.Context, dek []byte) (wrapped []byte, keyRef string, err error)
	Unwrap(ctx context.Context, wrapped []byte, keyRef string) ([]byte, error)
}

type Sealed struct {
	Ciphertext []byte
	Nonce      []byte
	WrappedDEK []byte
	KeyRef     string
}

type Envelope struct{ kw KeyWrapper }

func NewEnvelope(kw KeyWrapper) *Envelope { return &Envelope{kw: kw} }

// Seal encrypts plaintext. aad (e.g. the document ID) is authenticated but not
// encrypted, binding the ciphertext to its database record.
func (e *Envelope) Seal(ctx context.Context, plaintext, aad []byte) (*Sealed, error) {
	dek := make([]byte, 32)
	if _, err := rand.Read(dek); err != nil {
		return nil, err
	}
	defer zero(dek)
	ct, nonce, err := gcmSeal(dek, plaintext, aad)
	if err != nil {
		return nil, err
	}
	wrapped, ref, err := e.kw.Wrap(ctx, dek)
	if err != nil {
		return nil, fmt.Errorf("wrap data key: %w", err)
	}
	return &Sealed{Ciphertext: ct, Nonce: nonce, WrappedDEK: wrapped, KeyRef: ref}, nil
}

func (e *Envelope) Open(ctx context.Context, s *Sealed, aad []byte) ([]byte, error) {
	dek, err := e.kw.Unwrap(ctx, s.WrappedDEK, s.KeyRef)
	if err != nil {
		return nil, fmt.Errorf("unwrap data key: %w", err)
	}
	defer zero(dek)
	return gcmOpen(dek, s.Ciphertext, s.Nonce, aad)
}

func gcmSeal(key, plaintext, aad []byte) (ct, nonce []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	return gcm.Seal(nil, nonce, plaintext, aad), nonce, nil
}

func gcmOpen(key, ct, nonce, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ct, aad)
}

func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// ---------- Local KEK (development only) ----------

type LocalKEK struct{ kek []byte }

func NewLocalKEK(b64 string) (*LocalKEK, error) {
	k, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(k) != 32 {
		return nil, errors.New("LOCAL_KEK_BASE64 must be 32 random bytes, base64-encoded (openssl rand -base64 32)")
	}
	return &LocalKEK{kek: k}, nil
}

func (l *LocalKEK) Wrap(_ context.Context, dek []byte) ([]byte, string, error) {
	ct, nonce, err := gcmSeal(l.kek, dek, []byte("dek"))
	if err != nil {
		return nil, "", err
	}
	return append(nonce, ct...), "local:v1", nil
}

func (l *LocalKEK) Unwrap(_ context.Context, wrapped []byte, ref string) ([]byte, error) {
	if ref != "local:v1" {
		return nil, fmt.Errorf("unknown key ref %q", ref)
	}
	if len(wrapped) < 12 {
		return nil, errors.New("wrapped key too short")
	}
	return gcmOpen(l.kek, wrapped[12:], wrapped[:12], []byte("dek"))
}

// ---------- Vault / OpenBao transit (production) ----------

// VaultTransit wraps keys with the transit secrets engine. The KEK never leaves
// Vault, supports rotation (ciphertexts carry their key version), and every
// unwrap is audit-logged by Vault.
type VaultTransit struct {
	addr, token, key string
	http             *http.Client
}

func NewVaultTransit(addr, token, key string) *VaultTransit {
	return &VaultTransit{addr: strings.TrimRight(addr, "/"), token: token, key: key, http: &http.Client{Timeout: 10 * time.Second}}
}

func (v *VaultTransit) call(ctx context.Context, op string, body map[string]string) (map[string]any, error) {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/v1/transit/%s/%s", v.addr, op, v.key), bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Vault-Token", v.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := v.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vault transit %s: status %d", op, resp.StatusCode)
	}
	var out struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

func (v *VaultTransit) Wrap(ctx context.Context, dek []byte) ([]byte, string, error) {
	data, err := v.call(ctx, "encrypt", map[string]string{"plaintext": base64.StdEncoding.EncodeToString(dek)})
	if err != nil {
		return nil, "", err
	}
	ct, _ := data["ciphertext"].(string)
	if ct == "" {
		return nil, "", errors.New("vault returned no ciphertext")
	}
	return []byte(ct), "vault:" + v.key, nil
}

func (v *VaultTransit) Unwrap(ctx context.Context, wrapped []byte, ref string) ([]byte, error) {
	if ref != "vault:"+v.key {
		return nil, fmt.Errorf("unknown key ref %q", ref)
	}
	data, err := v.call(ctx, "decrypt", map[string]string{"ciphertext": string(wrapped)})
	if err != nil {
		return nil, err
	}
	pt, _ := data["plaintext"].(string)
	return base64.StdEncoding.DecodeString(pt)
}
