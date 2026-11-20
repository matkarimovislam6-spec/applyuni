package ocr

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// Engine turns an image or PDF into plain text, on this machine.
type Engine interface {
	Name() string
	Text(ctx context.Context, contentType string, data []byte) (string, error)
}

var ErrNoEngine = errors.New("no OCR engine available")

// Detect picks an engine: "vision" (macOS), "tesseract", or "auto" (Vision on
// macOS, else Tesseract if installed).
func Detect(pref string) (Engine, error) {
	pref = strings.ToLower(strings.TrimSpace(pref))
	if pref == "" {
		pref = "auto"
	}
	if pref == "vision" || (pref == "auto" && runtime.GOOS == "darwin") {
		if _, err := exec.LookPath("xcrun"); err == nil {
			return &Vision{}, nil
		}
		if pref == "vision" {
			return nil, fmt.Errorf("%w: Apple Vision needs macOS with the Xcode command line tools", ErrNoEngine)
		}
	}
	if pref == "tesseract" || pref == "auto" {
		if _, err := exec.LookPath("tesseract"); err == nil {
			return &Tesseract{Languages: "eng+rus+uzb+ita+deu"}, nil
		}
		return nil, fmt.Errorf("%w: install tesseract (apt install tesseract-ocr tesseract-ocr-rus poppler-utils)", ErrNoEngine)
	}
	return nil, fmt.Errorf("%w: unknown OCR_ENGINE %q", ErrNoEngine, pref)
}

// withFile writes data to a private temporary file for the engine to read.
func withFile(data []byte, ext string, fn func(path string) (string, error)) (string, error) {
	dir, err := os.MkdirTemp("", "applyuni-ocr-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	p := filepath.Join(dir, "doc"+ext)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return "", err
	}
	return fn(p)
}

func kind(contentType string) (string, string, error) {
	switch {
	case contentType == "application/pdf":
		return "pdf", ".pdf", nil
	case contentType == "image/jpeg":
		return "image", ".jpg", nil
	case contentType == "image/png":
		return "image", ".png", nil
	}
	return "", "", fmt.Errorf("OCR does not read %s", contentType)
}

func run(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	var out, errb bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %v: %s", filepath.Base(name), err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

// ---------- Apple Vision (macOS) ----------

//go:embed vision.swift
var visionSource []byte

// Vision uses macOS's built-in text recognition through a small Swift helper,
// compiled once into the user cache directory.
type Vision struct {
	once sync.Once
	bin  string
	err  error
}

func (v *Vision) Name() string { return "apple-vision" }

func (v *Vision) helper(ctx context.Context) (string, error) {
	v.once.Do(func() {
		sum := sha256.Sum256(visionSource)
		cache, err := os.UserCacheDir()
		if err != nil {
			v.err = err
			return
		}
		dir := filepath.Join(cache, "applyuni")
		v.bin = filepath.Join(dir, "ocr-vision-"+hex.EncodeToString(sum[:6]))
		if _, err := os.Stat(v.bin); err == nil {
			return
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			v.err = err
			return
		}
		src := v.bin + ".swift"
		if err := os.WriteFile(src, visionSource, 0o600); err != nil {
			v.err = err
			return
		}
		if _, err := run(ctx, "xcrun", "swiftc", "-O", "-o", v.bin, src); err != nil {
			v.err = fmt.Errorf("compile OCR helper: %w", err)
		}
	})
	return v.bin, v.err
}

func (v *Vision) Text(ctx context.Context, contentType string, data []byte) (string, error) {
	k, ext, err := kind(contentType)
	if err != nil {
		return "", err
	}
	bin, err := v.helper(ctx)
	if err != nil {
		return "", err
	}
	return withFile(data, ext, func(p string) (string, error) { return run(ctx, bin, p, k) })
}

// ---------- Tesseract (Linux servers) ----------

// Tesseract runs the tesseract CLI; PDFs are rendered with pdftoppm first.
type Tesseract struct {
	Languages string // e.g. "eng+rus"; languages that aren't installed are skipped
	once      sync.Once
	langs     string
}

func (t *Tesseract) Name() string { return "tesseract" }

func (t *Tesseract) languages(ctx context.Context) string {
	t.once.Do(func() {
		out, err := run(ctx, "tesseract", "--list-langs")
		have := map[string]bool{}
		if err == nil {
			for _, l := range strings.Split(out, "\n") {
				have[strings.TrimSpace(l)] = true
			}
		}
		var use []string
		for _, l := range strings.Split(t.Languages, "+") {
			if have[l] {
				use = append(use, l)
			}
		}
		if len(use) == 0 {
			use = []string{"eng"}
		}
		t.langs = strings.Join(use, "+")
	})
	return t.langs
}

func (t *Tesseract) Text(ctx context.Context, contentType string, data []byte) (string, error) {
	k, ext, err := kind(contentType)
	if err != nil {
		return "", err
	}
	langs := t.languages(ctx)
	return withFile(data, ext, func(p string) (string, error) {
		pages := []string{p}
		if k == "pdf" {
			if _, err := run(ctx, "pdftoppm", "-r", "300", "-l", "10", "-png", p, filepath.Join(filepath.Dir(p), "page")); err != nil {
				return "", fmt.Errorf("render PDF (install poppler-utils): %w", err)
			}
			pages, _ = filepath.Glob(filepath.Join(filepath.Dir(p), "page*.png"))
			sort.Strings(pages)
		}
		var texts []string
		for _, pg := range pages {
			out, err := run(ctx, "tesseract", pg, "stdout", "-l", langs)
			if err != nil {
				return "", err
			}
			texts = append(texts, out)
		}
		return strings.Join(texts, "\n\f\n"), nil
	})
}
