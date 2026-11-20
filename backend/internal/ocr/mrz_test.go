package ocr

import (
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)

// The ICAO 9303 specimen passport (Utopia, Anna Maria Eriksson).
const specimenTD3 = `P<UTOERIKSSON<<ANNA<MARIA<<<<<<<<<<<<<<<<<<<
L898902C36UTO7408122F1204159ZE184226B<<<<<10`

func TestICAOSpecimenPassport(t *testing.T) {
	text := "PASSPORT\nUtopia\nSome other OCR text\n" + specimenTD3 + "\n"
	m, err := FindMRZ(text, now)
	if err != nil {
		t.Fatal(err)
	}
	want := MRZ{Format: "TD3", DocumentCode: "P", IssuingState: "UTO", Surname: "ERIKSSON", GivenNames: "ANNA MARIA",
		DocumentNumber: "L898902C3", Nationality: "UTO", BirthDate: "1974-08-12", Sex: "F", ExpiryDate: "2012-04-15"}
	if *m != want {
		t.Fatalf("got %+v\nwant %+v", *m, want)
	}
	if m.FullName() != "ANNA MARIA ERIKSSON" {
		t.Fatal(m.FullName())
	}
}

// Typical OCR damage: spaces, « for <<, a dropped trailing filler, the
// letter O instead of zero in a date, lower case.
func TestDamagedOCR(t *testing.T) {
	text := "p<utoeriksson«anna<maria<<<<<<<<<<<<<<<<<<\n" +
		"L898902C36UTO74O8122F1204159ZE184226B<<<<< 10\n"
	m, err := FindMRZ(text, now)
	if err != nil {
		t.Fatal(err)
	}
	if m.DocumentNumber != "L898902C3" || m.BirthDate != "1974-08-12" || m.Surname != "ERIKSSON" {
		t.Fatalf("%+v", m)
	}
}

func TestWrongCheckDigitIsRefused(t *testing.T) {
	bad := strings.Replace(specimenTD3, "L898902C36", "L898902C37", 1)
	if _, err := FindMRZ(bad, now); err == nil {
		t.Fatal("accepted a wrong check digit")
	}
	// A misread name letter is caught by nothing, but a misread number is:
	bad = strings.Replace(specimenTD3, "7408122", "7408132", 1)
	if _, err := FindMRZ(bad, now); err == nil {
		t.Fatal("accepted a wrong birth date")
	}
}

// The ICAO 9303 specimen ID card (TD1).
func TestICAOSpecimenIDCard(t *testing.T) {
	text := "I<UTOD231458907<<<<<<<<<<<<<<<\n7408122F1204159UTO<<<<<<<<<<<6\nERIKSSON<<ANNA<MARIA<<<<<<<<<<\n"
	m, err := FindMRZ(text, now)
	if err != nil {
		t.Fatal(err)
	}
	if m.Format != "TD1" || m.DocumentCode != "I" || m.DocumentNumber != "D23145890" || m.GivenNames != "ANNA MARIA" {
		t.Fatalf("%+v", m)
	}
}

func TestNoMRZ(t *testing.T) {
	if _, err := FindMRZ("ATTESTAT\nUmumiy o'rta ta'lim\n", now); err == nil {
		t.Fatal("found an MRZ in plain text")
	}
}

func TestCenturies(t *testing.T) {
	if d, _ := mrzDate("040301", now, false); d != "2004-03-01" {
		t.Fatal(d)
	}
	if d, _ := mrzDate("990301", now, false); d != "1999-03-01" {
		t.Fatal(d)
	}
	if d, _ := mrzDate("340301", now, true); d != "2034-03-01" {
		t.Fatal(d)
	}
}
