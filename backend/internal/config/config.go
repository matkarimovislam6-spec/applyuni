package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	HTTPAddr    string
	DatabaseURL string
	JWTSecret   string
	CORSOrigin  string
	MaxUploadMB int64

	// Key management: "local" (dev only, KEK from env) or "vault" (OpenBao/Vault transit).
	KMSProvider   string
	LocalKEK      string // base64, 32 bytes
	VaultAddr     string
	VaultToken    string
	VaultTransKey string

	// Object storage: "local" (dev) or "s3" (MinIO / any S3-compatible, hosted in Uzbekistan).
	StorageProvider string
	LocalStorageDir string
	S3Endpoint      string
	S3AccessKey     string
	S3SecretKey     string
	S3Bucket        string
	S3UseSSL        bool

	// AI. Empty key disables AI analysis (documents stay "uploaded" for manual review).
	AnthropicAPIKey string
	AIModel         string

	RunWorker bool
}

func Load() (*Config, error) {
	c := &Config{
		HTTPAddr:        env("HTTP_ADDR", ":8080"),
		DatabaseURL:     env("DATABASE_URL", "sqlite:./data/applyuni.db"),
		JWTSecret:       os.Getenv("JWT_SECRET"),
		CORSOrigin:      env("CORS_ORIGIN", "http://localhost:5173"),
		MaxUploadMB:     int64(envInt("MAX_UPLOAD_MB", 20)),
		KMSProvider:     env("KMS_PROVIDER", "local"),
		LocalKEK:        os.Getenv("LOCAL_KEK_BASE64"),
		VaultAddr:       env("VAULT_ADDR", "http://localhost:8200"),
		VaultToken:      os.Getenv("VAULT_TOKEN"),
		VaultTransKey:   env("VAULT_TRANSIT_KEY", "applyuni-documents"),
		StorageProvider: env("STORAGE_PROVIDER", "local"),
		LocalStorageDir: env("LOCAL_STORAGE_DIR", "./data/objects"),
		S3Endpoint:      env("S3_ENDPOINT", "localhost:9000"),
		S3AccessKey:     os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey:     os.Getenv("S3_SECRET_KEY"),
		S3Bucket:        env("S3_BUCKET", "applyuni-documents"),
		S3UseSSL:        env("S3_USE_SSL", "false") == "true",
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		AIModel:         env("AI_MODEL", "claude-opus-5-5"),
		RunWorker:       env("RUN_WORKER", "true") == "true",
	}
	if len(c.JWTSecret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET must be set and at least 32 characters")
	}
	return c, nil
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(k)); err == nil {
		return v
	}
	return def
}
