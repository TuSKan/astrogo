package dust

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
)

// The service returns reddening, 100 micron emission and dust temperature in
// one document, each with identically named elements. Reading the first
// refPixelValue would return E(B-V) in magnitudes where an intensity in MJy/sr
// was wanted — a number of plausible size, in the wrong quantity, that nothing
// downstream could detect.
const sample = `<?xml version="1.0"?>
<results status="ok">
  <result>
    <desc>E(B-V) Reddening</desc>
    <statistics>
      <refPixelValueSFD>100.0269 (mag)</refPixelValueSFD>
      <refPixelValue>86.0231 (mag)</refPixelValue>
    </statistics>
  </result>
  <result>
    <desc>100 Micron Emission</desc>
    <statistics>
      <refPixelValue>17221.7422 (MJy/sr)</refPixelValue>
      <meanValue>16490.4246 (MJy/sr)</meanValue>
    </statistics>
  </result>
  <result>
    <desc>Dust Temperature</desc>
    <statistics>
      <refPixelValue>21.1993 (K)</refPixelValue>
    </statistics>
  </result>
</results>`

func TestParseTakesTheHundredMicronBlock(t *testing.T) {
	t.Parallel()

	got, err := parse(sample)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if math.Abs(got-17221.7422) > 1e-4 {
		t.Errorf("got %v, want the 100 micron value 17221.7422 — not the reddening", got)
	}
}

func TestParseRejectsWhatItCannotRead(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ name, doc string }{
		{"no 100 micron block", `<results><result><desc>E(B-V) Reddening</desc>
			<statistics><refPixelValue>1.5 (mag)</refPixelValue></statistics></result></results>`},
		{"wrong units", `<results><result><desc>100 Micron Emission</desc>
			<statistics><refPixelValue>1.5 (mag)</refPixelValue></statistics></result></results>`},
		{"empty", ""},
	} {
		if _, err := parse(tc.doc); !errors.Is(err, ErrResponse) {
			t.Errorf("%s: err = %v, want ErrResponse", tc.name, err)
		}
	}
}

// A direction never fetched is missing data, not a dust-free sightline. The
// component treats the two differently and only one of them is honest.
func TestUnfetchedDirectionReportsNoCoverage(t *testing.T) {
	t.Parallel()

	m := NewMap()

	if _, err := m.IntensityAt(angle.Deg(10), angle.Deg(20)); !errors.Is(err, ErrNoCoverage) {
		t.Errorf("err = %v, want ErrNoCoverage", err)
	}

	m.set(angle.Deg(10), angle.Deg(20), 3.5)

	got, err := m.IntensityAt(angle.Deg(10), angle.Deg(20))
	if err != nil {
		t.Fatalf("IntensityAt: %v", err)
	}

	if got != 3.5 {
		t.Errorf("got %v, want 3.5", got)
	}
}

// Nearby directions share a cell, so a target list clustered on one field
// costs one request rather than one per target.
func TestNearbyDirectionsShareACell(t *testing.T) {
	t.Parallel()

	m := NewMap()
	m.set(angle.Deg(120), angle.Deg(30), 1.25)

	// Well inside a tenth of a degree.
	if _, err := m.IntensityAt(angle.Deg(120.02), angle.Deg(30.01)); err != nil {
		t.Errorf("a direction 0.02 deg away should share the cell: %v", err)
	}

	// Well outside it.
	if _, err := m.IntensityAt(angle.Deg(120.5), angle.Deg(30)); !errors.Is(err, ErrNoCoverage) {
		t.Error("half a degree away is a different cell and must not be answered")
	}

	if m.Len() != 1 {
		t.Errorf("map holds %d cells, want 1", m.Len())
	}
}

// The map satisfies the interface the component consumes. This is a compile-
// time check written as a test so the failure names the reason.
func TestMapSatisfiesDustMap(t *testing.T) {
	t.Parallel()

	var _ interface {
		IntensityAt(l, b angle.Angle) (float64, error)
	} = NewMap()
}
