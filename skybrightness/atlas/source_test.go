package atlas

import (
	"bytes"
	"errors"
	"testing"
)

// TestSourcesCatalog verifies the catalog lists all three sources with the
// expected fidelity ordering and availability.
func TestSourcesCatalog(t *testing.T) {
	t.Parallel()

	got := Sources()
	if len(got) != 3 {
		t.Fatalf("Sources() len = %d, want 3", len(got))
	}

	// WorldAtlas and LightPollutionAtlas are propagated (fidelity 1); VIIRS is
	// raw radiance (fidelity 2 and not propagated).
	wa, lpa, viirs := got[WorldAtlas], got[LightPollutionAtlas], got[VIIRS]

	if !wa.Propagated || !lpa.Propagated || viirs.Propagated {
		t.Errorf("propagated flags wrong: WA=%v LPA=%v VIIRS=%v", wa.Propagated, lpa.Propagated, viirs.Propagated)
	}

	if wa.Fidelity != 1 || lpa.Fidelity != 1 || viirs.Fidelity <= wa.Fidelity {
		t.Errorf("fidelity order wrong: WA=%d LPA=%d VIIRS=%d", wa.Fidelity, lpa.Fidelity, viirs.Fidelity)
	}

	// Only LightPollutionAtlas lacks obtainable numeric data.
	if !wa.Available || lpa.Available || !viirs.Available {
		t.Errorf("availability wrong: WA=%v LPA=%v VIIRS=%v", wa.Available, lpa.Available, viirs.Available)
	}
}

// TestSourceInfoString verifies Info and String for valid and invalid sources.
func TestSourceInfoString(t *testing.T) {
	t.Parallel()

	if got := WorldAtlas.String(); got != "WA-2015" {
		t.Errorf("WorldAtlas.String() = %q, want WA-2015", got)
	}

	if _, err := Source(99).Info(); !errors.Is(err, ErrUnknownSource) {
		t.Errorf("Source(99).Info(): expected ErrUnknownSource, got %v", err)
	}

	if got := Source(99).String(); got != "Source(99)" {
		t.Errorf("Source(99).String() = %q, want Source(99)", got)
	}
}

// TestOpenGeoTIFFDispatch verifies the selector routes GeoTIFF-backed sources to
// working providers and reports the unavailable / unknown cases.
func TestOpenGeoTIFFDispatch(t *testing.T) {
	t.Parallel()

	// WorldAtlas pixel holds the natural-background luminance ⇒ ~22.0 mag.
	wa := synthTIFF{
		width: 2, height: 2,
		pixels:    []float32{0.171168465, 100, 50, 10},
		originLon: -47, originLat: -22, pxSize: 0.5,
	}

	p, err := OpenGeoTIFF(WorldAtlas, bytes.NewReader(wa.build(t)), nil)
	if err != nil {
		t.Fatalf("OpenGeoTIFF(WorldAtlas): %v", err)
	}

	lon, lat := wa.centerLonLat(0, 0)
	if sb, err := p.ZenithBrightness(lat, lon); err != nil || float64(sb) < 21 || float64(sb) > 23 {
		t.Errorf("WorldAtlas SB = %.2f (err %v), want ~22", float64(sb), err)
	}

	// VIIRS routes to the radiance provider (radiance grid, distinct values).
	viirs := synthTIFF{width: 2, height: 2, pixels: rampPixels(2, 2, 5), originLon: 0, originLat: 0, pxSize: 1}
	if _, err := OpenGeoTIFF(VIIRS, bytes.NewReader(viirs.build(t)), nil); err != nil {
		t.Errorf("OpenGeoTIFF(VIIRS): %v", err)
	}

	// LightPollutionAtlas has no numeric grid.
	if _, err := OpenGeoTIFF(LightPollutionAtlas, bytes.NewReader(nil), nil); !errors.Is(err, ErrSourceUnavailable) {
		t.Errorf("OpenGeoTIFF(LightPollutionAtlas): expected ErrSourceUnavailable, got %v", err)
	}

	// Unknown source.
	if _, err := OpenGeoTIFF(Source(99), bytes.NewReader(nil), nil); !errors.Is(err, ErrUnknownSource) {
		t.Errorf("OpenGeoTIFF(99): expected ErrUnknownSource, got %v", err)
	}
}
