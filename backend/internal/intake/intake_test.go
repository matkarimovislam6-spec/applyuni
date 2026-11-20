package intake

import (
	"bytes"
	"image"
	"image/png"
	"strings"
	"testing"
)

func pngOf(w, h int) []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewGray(image.Rect(0, 0, w, h)))
	return b.Bytes()
}

const pdf = "%PDF-1.4\n1 0 obj << /Type /Page >> endobj\n2 0 obj << /Type /Page >> endobj\ntrailer\n%%EOF\n"

func TestInspect(t *testing.T) {
	cases := []struct {
		name, file string
		data       []byte
		wantType   string
		wantErr    string
	}{
		{"pdf", "passport.pdf", []byte(pdf), "application/pdf", ""},
		{"png big enough", "scan.png", pngOf(1200, 900), "image/png", ""},
		{"png too small", "scan.png", pngOf(400, 300), "", "too small"},
		{"browser lies: pdf named .jpg", "photo.jpg", []byte(pdf), "application/pdf", ""},
		{"exe renamed pdf", "x.pdf", []byte("MZ\x90\x00 not a pdf"), "", "Only PDF"},
		{"encrypted pdf", "a.pdf", []byte("%PDF-1.7\n trailer << /Encrypt 5 0 R >>\n%%EOF"), "", "password-protected"},
		{"truncated pdf", "a.pdf", []byte("%PDF-1.7\n1 0 obj"), "", "incomplete"},
		{"empty", "a.pdf", nil, "", "empty"},
		{"heic", "IMG_1.HEIC", append([]byte{0, 0, 0, 24}, []byte("ftypheic0000")...), "", "HEIC"},
	}
	for _, c := range cases {
		info, err := Inspect(c.data, c.file)
		if c.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), c.wantErr) || !IsProblem(err) {
				t.Errorf("%s: want problem %q, got %v", c.name, c.wantErr, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: unexpected %v", c.name, err)
			continue
		}
		if info.ContentType != c.wantType {
			t.Errorf("%s: type %s, want %s", c.name, info.ContentType, c.wantType)
		}
	}
	if info, _ := Inspect([]byte(pdf), "a.pdf"); info.Pages != 2 {
		t.Errorf("pages = %d, want 2", info.Pages)
	}
}

func TestSafeFilename(t *testing.T) {
	for in, want := range map[string]string{
		"../../etc/passwd":        "passwd",
		`C:\Users\me\pasport.pdf`: "pasport.pdf",
		"Паспорт Ислом.pdf":       "Паспорт Ислом.pdf",
		"a<script>.pdf":           "a_script_.pdf",
		"":                        "document",
		".hidden":                 "hidden",
	} {
		if got := SafeFilename(in); got != want {
			t.Errorf("SafeFilename(%q) = %q, want %q", in, got, want)
		}
	}
}
