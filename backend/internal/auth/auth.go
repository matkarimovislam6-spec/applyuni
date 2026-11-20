package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"
)

// ---------- Passwords (argon2id) ----------

const (
	argonTime    = 2
	argonMemory  = 64 * 1024
	argonThreads = 2
	argonKeyLen  = 32
)

func HashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(pw), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("argon2id$%d$%d$%d$%s$%s", argonTime, argonMemory, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func CheckPassword(hash, pw string) bool {
	var t, m uint32
	var p uint8
	var saltB64, keyB64 string
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[0] != "argon2id" {
		return false
	}
	if _, err := fmt.Sscanf(strings.Join(parts[1:4], " "), "%d %d %d", &t, &m, &p); err != nil {
		return false
	}
	saltB64, keyB64 = parts[4], parts[5]
	salt, err1 := base64.RawStdEncoding.DecodeString(saltB64)
	key, err2 := base64.RawStdEncoding.DecodeString(keyB64)
	if err1 != nil || err2 != nil {
		return false
	}
	got := argon2.IDKey([]byte(pw), salt, t, m, p, uint32(len(key)))
	return subtle.ConstantTimeCompare(got, key) == 1
}

// ---------- Tokens ----------

type Claims struct {
	UserID uuid.UUID `json:"uid"`
	OrgID  uuid.UUID `json:"oid"`
	Role   string    `json:"role"`
	jwt.RegisteredClaims
}

// SessionTTL is how long a login is remembered.
const SessionTTL = 30 * 24 * time.Hour

// CookieName holds the session token. It is HttpOnly, so page scripts
// (and any injected script) can't read it.
const CookieName = "applyuni_session"

type Issuer struct{ secret []byte }

func NewIssuer(secret string) *Issuer { return &Issuer{secret: []byte(secret)} }

func (i *Issuer) Issue(userID, orgID uuid.UUID, role string) (string, error) {
	c := Claims{UserID: userID, OrgID: orgID, Role: role, RegisteredClaims: jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(SessionTTL)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(i.secret)
}

func (i *Issuer) Parse(tok string) (*Claims, error) {
	c := &Claims{}
	_, err := jwt.ParseWithClaims(tok, c, func(t *jwt.Token) (any, error) { return i.secret, nil },
		jwt.WithValidMethods([]string{"HS256"}))
	if err != nil {
		return nil, err
	}
	return c, nil
}

type ctxKey struct{}

func (i *Issuer) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The browser sends the HttpOnly cookie; API clients may use a Bearer header.
		tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			ck, err := r.Cookie(CookieName)
			if err != nil || ck.Value == "" {
				w.Header().Set("Content-Type", "application/json")
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			tok = ck.Value
		}
		c, err := i.Parse(tok)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, c)))
	})
}

func FromContext(ctx context.Context) *Claims {
	c, _ := ctx.Value(ctxKey{}).(*Claims)
	return c
}

var ErrForbidden = errors.New("forbidden")

// IsStaff reports whether the caller manages students in a B2B org.
func (c *Claims) IsStaff() bool { return c.Role == "owner" || c.Role == "counselor" }
