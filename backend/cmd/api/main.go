package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/matkarimovislam6-spec/applyuni/backend/internal/ai"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/api"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/auth"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/config"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/crypto"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/db"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/portal"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/service"
	"github.com/matkarimovislam6-spec/applyuni/backend/internal/storage"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	database, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	if err := database.Migrate(ctx); err != nil {
		return err
	}

	var kw crypto.KeyWrapper
	switch cfg.KMSProvider {
	case "vault":
		kw = crypto.NewVaultTransit(cfg.VaultAddr, cfg.VaultToken, cfg.VaultTransKey)
	case "local":
		log.Warn("using LOCAL key encryption key - development only")
		if kw, err = crypto.NewLocalKEK(cfg.LocalKEK); err != nil {
			return err
		}
	default:
		return errors.New("KMS_PROVIDER must be 'local' or 'vault'")
	}

	var store storage.Store
	switch cfg.StorageProvider {
	case "s3":
		store, err = storage.NewS3(ctx, cfg.S3Endpoint, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Bucket, cfg.S3UseSSL)
	default:
		store, err = storage.NewLocal(cfg.LocalStorageDir)
	}
	if err != nil {
		return err
	}

	// Leave the interface nil (not a typed nil pointer) when AI is off.
	var aiClient service.Analyzer
	if cfg.AnthropicAPIKey != "" {
		aiClient = ai.New(cfg.AnthropicAPIKey, cfg.AIModel)
	} else {
		log.Warn("ANTHROPIC_API_KEY not set - AI document checks disabled, documents need manual review")
	}

	tokens := auth.NewIssuer(cfg.JWTSecret)
	svc := &service.Service{
		DB: database, Env: crypto.NewEnvelope(kw), Store: store, AI: aiClient,
		Tokens: tokens, MailDomain: envOr("MAIL_DOMAIN", "inbox.applyuni.local"), InboundSecret: cfg.InboundSecret, Log: log,
	}
	// The portal agent reads verification codes from the students' platform inboxes.
	svc.Portals = portals(log, svc.Inbox())
	if cfg.InboundSecret == "" {
		log.Warn("INBOUND_SECRET not set - inbound email disabled; portals that email a code can't be automated")
	}
	if cfg.RunWorker {
		go svc.RunWorker(ctx)
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.New(svc, tokens, cfg.CORSOrigin, cfg.MaxUploadMB, cfg.CookieSecure, log),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Info("listening", "addr", cfg.HTTPAddr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// portals registers the real university flows (portal.Known) with a headless
// Chrome runner. Universities without a flow fall back to the dry-run adapter
// in development (PORTAL_DRY_RUN=true) or are parked with a clear note.
func portals(log *slog.Logger, inbox portal.Inbox) *portal.Registry {
	reg := portal.NewRegistry(os.Getenv("PORTAL_DRY_RUN") == "true")
	runner := &portal.Runner{
		ChromePath: os.Getenv("CHROME_PATH"), // empty: find Chrome on PATH
		Headless:   true,
		Inbox:      inbox,
	}
	for _, f := range portal.Known {
		if err := f.Validate(); err != nil {
			log.Error("skipping invalid portal flow", "portal", f.Portal, "err", err)
			continue
		}
		reg.Register(f.Portal, portal.FlowAdapter{Flow: f, Runner: runner})
		log.Info("portal flow registered", "portal", f.Portal)
	}
	return reg
}
