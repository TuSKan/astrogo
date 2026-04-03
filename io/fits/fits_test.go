package fits

import (
	"bytes"
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

	if err := VerifyPrimaryHeader(h); err != nil {
		t.Errorf("expected header to pass verification, got: %v", err)
	}

	h2 := NewHeader()
	h2.Append(Card{Keyword: "BITPIX", Value: "8"})
	h2.Append(Card{Keyword: "SIMPLE", Value: "T"})

	if err := VerifyPrimaryHeader(h2); err == nil {
		t.Errorf("expected failure due to SIMPLE not being first")
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
