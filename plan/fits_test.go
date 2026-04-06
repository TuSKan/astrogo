package plan

import (
	"testing"

	"github.com/TuSKan/astrogo/fits"
)

func TestSiteFromFITS(t *testing.T) {
	h := fits.NewHeader()
	h.Append(fits.Card{Keyword: "SITELONG", Value: "149.0661"}) // Siding Spring
	h.Append(fits.Card{Keyword: "SITELAT", Value: "-31.2770"})
	h.Append(fits.Card{Keyword: "SITEELEV", Value: "1165.0"})
	h.Append(fits.Card{Keyword: "OBSERVAT", Value: "'AAO'"})

	site, err := SiteFromFITS(h)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if site.Name() != "AAO" {
		t.Errorf("expected site name AAO, got: %s", site.Name())
	}
	if site.Longitude().Degrees() != 149.0661 {
		t.Errorf("expected longitude 149.0661, got: %v", site.Longitude().Degrees())
	}
	if site.Latitude().Degrees() != -31.2770 {
		t.Errorf("expected latitude -31.2770, got: %v", site.Latitude().Degrees())
	}
	if site.HeightMeters() != 1165.0 {
		t.Errorf("expected elevation 1165.0, got: %v", site.HeightMeters())
	}
}

func TestTargetFromFITS(t *testing.T) {
	h := fits.NewHeader()
	h.Append(fits.Card{Keyword: "OBJECT", Value: "'M42'"})
	h.Append(fits.Card{Keyword: "CRVAL1", Value: "83.82208"})
	h.Append(fits.Card{Keyword: "CRVAL2", Value: "-5.39111"})

	target, err := TargetFromFITS(h)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if target.Name() != "M42" {
		t.Errorf("expected name M42, got: %s", target.Name())
	}

	// test RA and Dec
	if target.Coord.RA().Degrees() != 83.82208 {
		t.Errorf("expected RA 83.82208, got: %v", target.Coord.RA().Degrees())
	}
	if target.Coord.Dec().Degrees() != -5.39111 {
		t.Errorf("expected Dec -5.39111, got: %v", target.Coord.Dec().Degrees())
	}
}
