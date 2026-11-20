package ocr

import (
	"context"
	"errors"
	"os"
	"testing"
)

// Real OCR on a synthetic page with the ICAO specimen data (a fictional
// person). Skipped where no OCR engine is installed.
func TestEngineReadsSpecimenPassport(t *testing.T) {
	eng, err := Detect("auto")
	if errors.Is(err, ErrNoEngine) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	img, err := os.ReadFile("testdata/specimen_passport.png")
	if err != nil {
		t.Fatal(err)
	}
	text, err := eng.Text(context.Background(), "image/png", img)
	if err != nil {
		t.Fatalf("%s: %v", eng.Name(), err)
	}
	m, err := FindMRZ(text, now)
	if err != nil {
		t.Fatalf("%s read:\n%s\nMRZ: %v", eng.Name(), text, err)
	}
	if m.DocumentNumber != "L898902C3" || m.BirthDate != "1974-08-12" || m.FullName() != "ANNA MARIA ERIKSSON" {
		t.Fatalf("%+v", m)
	}
}
