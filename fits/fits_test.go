package fits

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"testing"
)

func TestParseCard(t *testing.T) {
	cardStr := "SIMPLE  =                    T / file does conform to FITS standard             "

	c := ParseCard([]byte(cardStr))
	if c.Keyword != "SIMPLE" {
		t.Errorf("expected Keyword='SIMPLE', got '%s'", c.Keyword)
	}

	if c.Value != "T" {
		t.Errorf("expected Value='T', got '%s'", c.Value)
	}

	if c.Comment != "file does conform to FITS standard" {
		t.Errorf("expected Comment='file does conform to FITS standard', got '%s'", c.Comment)
	}

	// Test string
	cardStr2 := "INSTRUME= 'VLT-MUSE'           / Instrument used                                "

	c2 := ParseCard([]byte(cardStr2))
	if c2.Keyword != "INSTRUME" {
		t.Errorf("Expected INSTRUME, got '%s'", c2.Keyword)
	}

	if c2.Value != "'VLT-MUSE'" {
		t.Errorf("Expected 'VLT-MUSE', got '%s'", c2.Value)
	}
}

func TestVerifyPrimaryHeader(t *testing.T) {
	h := NewHeader()
	h.Append(Card{Keyword: "SIMPLE", Value: "T"})
	h.Append(Card{Keyword: "BITPIX", Value: "8"})
	h.Append(Card{Keyword: "NAXIS", Value: "0"})

	err := VerifyPrimaryHeader(h)
	if err != nil {
		t.Errorf("expected header to pass verification, got: %v", err)
	}

	h2 := NewHeader()
	h2.Append(Card{Keyword: "BITPIX", Value: "8"})
	h2.Append(Card{Keyword: "SIMPLE", Value: "T"})

	err = VerifyPrimaryHeader(h2)
	if err == nil {
		t.Errorf("expected failure due to SIMPLE not being first")
	}
}

func TestFitsOpenRead(t *testing.T) {
	// Let's create a temporary fits file
	tmp, err := os.CreateTemp(t.TempDir(), "test_fits_*.fits")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	headerData := make([]byte, 2880)
	for i := range 2880 {
		headerData[i] = ' '
	}

	copy(headerData, []byte(fmt.Sprintf("%-80s", "SIMPLE  =                    T / ")))
	copy(headerData[80:], []byte(fmt.Sprintf("%-80s", "BITPIX  =                    8 / ")))
	copy(headerData[160:], []byte(fmt.Sprintf("%-80s", "NAXIS   =                    0 / ")))
	copy(headerData[240:], []byte(fmt.Sprintf("%-80s", "END")))

	if _, err := tmp.Write(headerData); err != nil {
		t.Fatalf("failed to write header: %v", err)
	}

	if err := tmp.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}

	// Test Open
	f, err := Open(tmp.Name())
	if err != nil {
		t.Fatalf("failed Open: %v", err)
	}

	if len(f.HDUs) != 1 {
		t.Errorf("expected 1 HDU, got %d", len(f.HDUs))
	}

	// HDU interface tests to boost hdu.go
	hdu := f.HDUs[0]
	if hdu.Header() == nil {
		t.Errorf("expected Header to not be nil")
	}

	if hdu.Type() != HDUTypeImage {
		t.Errorf("expected HDUTypeImage")
	}

	// Invalid Open
	_, err = Open("does_not_exist_at_all.fits")
	if err == nil {
		t.Errorf("expected error on nonexistent file")
	}
}

func TestHeaderTypes(t *testing.T) {
	h := NewHeader()
	h.Append(Card{Keyword: "BITPIX", Value: "-32"})
	h.Append(Card{Keyword: "CRVAL1", Value: "21.56"})
	h.Append(Card{Keyword: "TELESCOP", Value: " 'HUBBLE' "})

	b, err := h.GetInt("BITPIX")
	if err != nil || b != -32 {
		t.Errorf("GetInt failed: %v, %v", b, err)
	}

	f, err := h.GetFloat("CRVAL1")
	if err != nil || f != 21.56 {
		t.Errorf("GetFloat failed: %v, %v", f, err)
	}

	s, err := h.GetString("TELESCOP")
	if err != nil || s != "HUBBLE" {
		t.Errorf("GetString failed: %v, %v", s, err)
	}
}

func TestBlockReader(t *testing.T) {
	data := make([]byte, 2880)
	copy(data[0:80], "SIMPLE  =                    T /                                                ")
	copy(data[80:160], "END                                                                             ")

	br := NewBlockReader(bytes.NewReader(data))

	h, err := ReadHeader(br)
	if err != nil {
		t.Fatalf("ReadHeader failed: %v", err)
	}

	c, _ := h.Get("SIMPLE")
	if c.Value != "T" {
		t.Errorf("Expected 'T', got %s", c.Value)
	}
}

func TestReadBigEndian(t *testing.T) {
	var val int32

	r := bytes.NewReader([]byte{0, 0, 0, 42})

	err := ReadBigEndian(r, &val)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}

	if val != 42 {
		t.Errorf("expected 42, got %d", val)
	}

	err = Write("dummy", nil)
	if !errors.Is(err, ErrUnimplemented) {
		t.Errorf("expected ErrUnimplemented, got %v", err)
	}
}

func TestVerifyError(t *testing.T) {
	ve := VerifyError{Keyword: "NAXIS", Reason: "missing"}
	if ve.Error() != "fits verify: keyword NAXIS missing" {
		t.Errorf("VerifyError format mismatch, got %s", ve.Error())
	}

	h := NewHeader()
	h.Append(Card{Keyword: "SIMPLE", Value: "F"})

	err := VerifyPrimaryHeader(h)
	if err == nil {
		t.Errorf("expected error for SIMPLE=F")
	}

	h = NewHeader()
	h.Append(Card{Keyword: "SIMPLE", Value: "T"})

	if err = VerifyPrimaryHeader(h); err == nil {
		t.Errorf("expected error for missing BITPIX")
	}

	h.Append(Card{Keyword: "BITPIX", Value: "8"})

	if err = VerifyPrimaryHeader(h); err == nil {
		t.Errorf("expected error for missing NAXIS")
	}
}

func TestHDU_Load(t *testing.T) {
	b := &basicHDU{}

	err := b.Load(nil)
	if err != nil {
		t.Errorf("expected nil from basicHDU Load")
	}
}
