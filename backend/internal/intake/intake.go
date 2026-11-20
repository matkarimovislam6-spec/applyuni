// Package intake is the first gate for every uploaded file. It trusts nothing
// the browser says: the real type comes from the file's own bytes, and files
// that can't be read later (password-protected PDFs, tiny photos) are turned
// away at upload, with a reason the student can act on.
package intake

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // register decoders for DecodeConfig
	_ "image/png"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Limits. Phone photos of documents are usually 1–5 MB; scans up to ~15 MB.
const (
	MaxBytes      = 20 << 20
	MinImageSide  = 700 // shortest side in pixels; below this text is rarely readable
	MaxPDFPages   = 60
	maxNameLength = 120
)

// Problem is a rejection the student can fix. Its message is shown as is.
type Problem struct{ Msg string }

func (p *Problem) Error() string { return p.Msg }

func problem(format string, a ...any) error { return &Problem{Msg: fmt.Sprintf(format, a...)} }

// IsProblem reports whether err is a student-fixable intake rejection.
func IsProblem(err error) bool {
	var p *Problem
	return errors.As(err, &p)
}

// Info is what we learned about an accepted file.
type Info struct {
	ContentType string // from the bytes, never from the browser
	Filename    string // safe to store and show
	Pages       int    // PDFs only (best effort)
	Width       int    // images only
	Height      int
}

// Inspect decides whether a file is acceptable and what it really is.
func Inspect(data []byte, filename string) (*Info, error) {
	if len(data) == 0 {
		return nil, problem("The file is empty.")
	}
	if len(data) > MaxBytes {
		return nil, problem("The file is larger than %d MB. Please upload a smaller scan.", MaxBytes>>20)
	}
	info := &Info{ContentType: Sniff(data), Filename: SafeFilename(filename)}
	switch info.ContentType {
	case "application/pdf":
		if err := checkPDF(data, info); err != nil {
			return nil, err
		}
	case "image/jpeg", "image/png":
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return nil, problem("The image is damaged and can't be opened. Please take the photo again.")
		}
		info.Width, info.Height = cfg.Width, cfg.Height
		if min(cfg.Width, cfg.Height) < MinImageSide {
			return nil, problem("The photo is too small to read (%d×%d pixels). Please upload a sharper photo or a scan.", cfg.Width, cfg.Height)
		}
	case "image/webp":
		// accepted; size is checked by the AI's legibility check
	case "image/heic":
		return nil, problem("iPhone HEIC photos aren't supported yet. Please change the camera format to \"Most Compatible\" or upload a PDF.")
	default:
		return nil, problem("Only PDF, JPEG, PNG or WEBP files are accepted.")
	}
	return info, nil
}

// Sniff returns the content type from the file's first bytes.
func Sniff(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte("%PDF-")):
		return "application/pdf"
	case bytes.HasPrefix(b, []byte{0xFF, 0xD8, 0xFF}):
		return "image/jpeg"
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case len(b) >= 12 && bytes.Equal(b[0:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")):
		return "image/webp"
	case len(b) >= 12 && bytes.Equal(b[4:8], []byte("ftyp")) && (bytes.Equal(b[8:12], []byte("heic")) || bytes.Equal(b[8:12], []byte("heix")) || bytes.Equal(b[8:12], []byte("mif1"))):
		return "image/heic"
	}
	return "application/octet-stream"
}

var (
	pageRe    = regexp.MustCompile(`/Type\s*/Page[^s]`)
	encryptRe = regexp.MustCompile(`/Encrypt\s`)
)

func checkPDF(data []byte, info *Info) error {
	if !bytes.Contains(data[max(0, len(data)-2048):], []byte("%%EOF")) {
		return problem("The PDF looks incomplete (the upload may have been cut off). Please upload it again.")
	}
	if encryptRe.Match(data) {
		return problem("The PDF is password-protected. Please upload a copy without a password.")
	}
	info.Pages = len(pageRe.FindAll(data, -1))
	if info.Pages > MaxPDFPages {
		return problem("The PDF has %d pages. Please upload only the pages of this document.", info.Pages)
	}
	return nil
}

var unsafeChars = regexp.MustCompile(`[^\p{L}\p{N}._ -]+`)

// SafeFilename keeps a readable name without paths or control characters.
func SafeFilename(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if !utf8.ValidString(name) {
		name = strings.ToValidUTF8(name, "")
	}
	name = strings.TrimSpace(unsafeChars.ReplaceAllString(name, "_"))
	name = strings.TrimLeft(name, ".")
	if name == "" || name == "_" {
		name = "document"
	}
	if r := []rune(name); len(r) > maxNameLength {
		ext := filepath.Ext(name)
		name = string(r[:maxNameLength-len([]rune(ext))]) + ext
	}
	return name
}
