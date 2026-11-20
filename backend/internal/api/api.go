// Package api is the HTTP layer: routing, decoding, error mapping.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/google/uuid"

	"github.com/matkarimovislam6-spec/applyuni/backend/internal/auth"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/requirements"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/service"
)

type API struct {
	svc           *service.Service
	tokens        *auth.Issuer
	maxUpload     int64
	secureCookies bool
	loginsByUser  *limiter // 10 failures per username per 15 min
	loginsByIP    *limiter // 50 per IP: many mobile users share one IP
	log           *slog.Logger
}

// New builds the HTTP handler. secureCookies must be true when served over HTTPS.
func New(svc *service.Service, tokens *auth.Issuer, corsOrigin string, maxUploadMB int64, secureCookies bool, log *slog.Logger) http.Handler {
	a := &API{svc: svc, tokens: tokens, maxUpload: maxUploadMB << 20, secureCookies: secureCookies,
		loginsByUser: newLimiter(10, 15*time.Minute), loginsByIP: newLimiter(50, 15*time.Minute), log: log}
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer, middleware.Timeout(60*time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{corsOrigin},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Content-Disposition"},
		AllowCredentials: true, // the session cookie
	}))
	r.Use(securityHeaders)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	r.Route("/api", func(r chi.Router) {
		r.Post("/auth/register", a.register)
		r.Post("/auth/login", a.login)
		r.Post("/auth/logout", a.logout)
		r.Get("/catalog/programs", a.listPrograms)
		r.Post("/catalog/programs/{id}/checklist", a.publicChecklist)
		r.Post("/waitlist", a.joinWaitlist)

		r.Group(func(r chi.Router) {
			r.Use(tokens.Middleware)
			r.Get("/me", a.me)
			r.Get("/me/matches", a.matches)
			r.Get("/students", a.listStudents)
			r.Post("/students", a.createStudent)
			r.Get("/students/{id}", a.getStudent)
			r.Put("/students/{id}/profile", a.updateProfile)
			r.Delete("/students/{id}", a.deleteStudent)
			r.Get("/students/{id}/applications", a.listApplications)
			r.Post("/students/{id}/applications", a.createApplication)
			r.Get("/students/{id}/documents", a.listDocuments)
			r.Post("/students/{id}/documents", a.uploadDocument)
			r.Get("/applications/{id}", a.getApplication)
			r.Post("/applications/{id}/approve", a.approve)
			r.Get("/documents/{id}/download", a.downloadDocument)
			r.Post("/documents/{id}/review", a.reviewDocument)
			r.Delete("/documents/{id}", a.deleteDocument)
		})
	})
	return r
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (a *API) fail(w http.ResponseWriter, r *http.Request, err error) {
	status, msg := http.StatusInternalServerError, "internal error"
	switch {
	case errors.Is(err, service.ErrNotFound):
		status, msg = http.StatusNotFound, "not found"
	case errors.Is(err, service.ErrForbidden):
		status, msg = http.StatusForbidden, "forbidden"
	case errors.Is(err, service.ErrBadLogin):
		status, msg = http.StatusUnauthorized, err.Error()
	case errors.Is(err, service.ErrInvalid), errors.Is(err, service.ErrConflict),
		errors.Is(err, service.ErrSeatLimit), errors.Is(err, service.ErrNotReady):
		status, msg = http.StatusUnprocessableEntity, publicMessage(err)
		if errors.Is(err, service.ErrConflict) {
			status = http.StatusConflict
		}
	default:
		a.log.Error("request failed", "path", r.URL.Path, "err", err)
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%w: malformed JSON", service.ErrInvalid)
	}
	return nil
}

func idParam(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return id, service.ErrNotFound
	}
	return id, nil
}

func claims(r *http.Request) *auth.Claims { return auth.FromContext(r.Context()) }

// ---------- handlers ----------

func (a *API) setSession(w http.ResponseWriter, token string, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(maxAge.Seconds()),
		HttpOnly: true,                 // not readable by page scripts
		Secure:   a.secureCookies,      // HTTPS only in production
		SameSite: http.SameSiteLaxMode, // not sent on cross-site POSTs (CSRF)
	})
}

func (a *API) register(w http.ResponseWriter, r *http.Request) {
	var in service.RegisterInput
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	s, err := a.svc.Register(r.Context(), in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.setSession(w, s.Token, auth.SessionTTL)
	writeJSON(w, http.StatusCreated, s)
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip == "" {
		ip = r.RemoteAddr
	}
	user := strings.ToLower(strings.TrimSpace(in.Username))
	if !a.loginsByUser.allowed(user) || !a.loginsByIP.allowed(ip) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many failed attempts, try again in 15 minutes"})
		return
	}
	s, err := a.svc.Login(r.Context(), in.Username, in.Password)
	if err != nil {
		if errors.Is(err, service.ErrBadLogin) {
			a.loginsByUser.fail(user)
			a.loginsByIP.fail(ip)
		}
		a.fail(w, r, err)
		return
	}
	a.loginsByUser.reset(user)
	a.setSession(w, s.Token, auth.SessionTTL)
	writeJSON(w, http.StatusOK, s)
}

func (a *API) logout(w http.ResponseWriter, _ *http.Request) {
	a.setSession(w, "", -time.Second) // expire the cookie
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	m, err := a.svc.Me(r.Context(), claims(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (a *API) matches(w http.ResponseWriter, r *http.Request) {
	m, err := a.svc.Matches(r.Context(), claims(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (a *API) listPrograms(w http.ResponseWriter, r *http.Request) {
	p, err := a.svc.ListPrograms(r.Context(), r.URL.Query().Get("country"), r.URL.Query().Get("degree"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (a *API) publicChecklist(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var prof requirements.Profile
	if err := decode(r, &prof); err != nil {
		a.fail(w, r, err)
		return
	}
	p, err := a.svc.PublicChecklist(r.Context(), id, prof)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (a *API) joinWaitlist(w http.ResponseWriter, r *http.Request) {
	var in service.WaitlistInput
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if err := a.svc.JoinWaitlist(r.Context(), in); err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
}

func (a *API) listStudents(w http.ResponseWriter, r *http.Request) {
	s, err := a.svc.ListStudents(r.Context(), claims(r))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (a *API) createStudent(w http.ResponseWriter, r *http.Request) {
	var in struct {
		FullName string `json:"full_name"`
		Email    string `json:"email"`
	}
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	s, err := a.svc.CreateStudent(r.Context(), claims(r), in.FullName, in.Email)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, s)
}

func (a *API) getStudent(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err == nil {
		var s *service.Student
		if s, err = a.svc.GetStudent(r.Context(), claims(r), id); err == nil {
			writeJSON(w, http.StatusOK, s)
			return
		}
	}
	a.fail(w, r, err)
}

func (a *API) updateProfile(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var profile json.RawMessage
	if err := decode(r, &profile); err != nil {
		a.fail(w, r, err)
		return
	}
	s, err := a.svc.UpdateProfile(r.Context(), claims(r), id, profile)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (a *API) deleteStudent(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err == nil {
		if err = a.svc.DeleteStudent(r.Context(), claims(r), id); err == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	a.fail(w, r, err)
}

func (a *API) listApplications(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err == nil {
		var apps []*service.Application
		if apps, err = a.svc.ListApplications(r.Context(), claims(r), id); err == nil {
			writeJSON(w, http.StatusOK, apps)
			return
		}
	}
	a.fail(w, r, err)
}

func (a *API) createApplication(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in struct {
		ProgramID uuid.UUID `json:"program_id"`
	}
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	app, err := a.svc.CreateApplication(r.Context(), claims(r), id, in.ProgramID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, app)
}

func (a *API) getApplication(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err == nil {
		var d *service.ApplicationDetail
		if d, err = a.svc.GetApplication(r.Context(), claims(r), id); err == nil {
			writeJSON(w, http.StatusOK, d)
			return
		}
	}
	a.fail(w, r, err)
}

func (a *API) approve(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in struct {
		Confirm bool `json:"confirm"`
	}
	if err := decode(r, &in); err != nil || !in.Confirm {
		a.fail(w, r, fmt.Errorf("%w: explicit confirmation is required", service.ErrInvalid))
		return
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip == "" {
		ip = r.RemoteAddr
	}
	d, err := a.svc.Approve(r.Context(), claims(r), id, ip, r.UserAgent())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (a *API) listDocuments(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err == nil {
		var docs []*service.Document
		if docs, err = a.svc.ListDocuments(r.Context(), claims(r), id); err == nil {
			writeJSON(w, http.StatusOK, docs)
			return
		}
	}
	a.fail(w, r, err)
}

func (a *API) uploadDocument(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, a.maxUpload+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		a.fail(w, r, fmt.Errorf("%w: file too large or malformed upload (max %d MB)", service.ErrInvalid, a.maxUpload>>20))
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		a.fail(w, r, fmt.Errorf("%w: missing file", service.ErrInvalid))
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, a.maxUpload+1))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if int64(len(data)) > a.maxUpload {
		a.fail(w, r, fmt.Errorf("%w: file larger than %d MB", service.ErrInvalid, a.maxUpload>>20))
		return
	}
	// Trust the bytes, not the client's header.
	ct := http.DetectContentType(data)
	if mt, _, err := mime.ParseMediaType(ct); err == nil {
		ct = mt
	}
	doc, err := a.svc.UploadDocument(r.Context(), claims(r), id, r.FormValue("doc_type"), filepath.Base(hdr.Filename), ct, data)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, doc)
}

func (a *API) downloadDocument(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	doc, data, err := a.svc.DownloadDocument(r.Context(), claims(r), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", doc.ContentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": doc.Filename}))
	w.Write(data)
}

func (a *API) reviewDocument(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in struct {
		Accept bool   `json:"accept"`
		Note   string `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	d, err := a.svc.ReviewDocument(r.Context(), claims(r), id, in.Accept, in.Note)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (a *API) deleteDocument(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err == nil {
		if err = a.svc.DeleteDocument(r.Context(), claims(r), id); err == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	a.fail(w, r, err)
}

// publicMessage drops the internal category prefix ("invalid request: ",
// "conflict: ") so users see a plain sentence.
func publicMessage(err error) string {
	msg := err.Error()
	for _, sentinel := range []error{service.ErrInvalid, service.ErrConflict} {
		msg = strings.TrimPrefix(msg, sentinel.Error()+": ")
	}
	return msg
}
