package plan_test

import (
	"errors"
	"math"
	"strings"
	"testing"
	gotime "time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/optics"
	sbplan "github.com/TuSKan/astrogo/skybrightness/plan"
)

// The sky calibrates itself: a source producing exactly the sky's per-pixel
// rate comes out at exactly the sky's per-pixel magnitude.
//
// # Why this is the anchoring case
//
// Because it is the one value the conversion cannot get wrong by a scale
// factor and still pass. Every other property here — the 2.5 log ratio, the
// pixel-area term — is a shape, and a shape can be right while the zero point
// is off by any constant. Setting signal equal to background collapses the
// ratio term to zero and leaves only the calibration, so if the pixel
// magnitude is wrong this is where it shows.
func TestLimitingMagnitudeAnchorsOnTheSkyItself(t *testing.T) {
	t.Parallel()

	const (
		surface      = 21.5 // mag/arcsec^2
		pixelArcsec2 = 4.0  // a 2" pixel
		background   = 12.3 // e/pixel/s
	)

	got := sbplan.LimitingMagnitude(surface, pixelArcsec2, background, background)
	want := surface - 2.5*math.Log10(pixelArcsec2)

	if math.Abs(got-want) > 1e-12 {
		t.Errorf("a source at the sky's own rate is %.12f, want the sky's own per-pixel "+
			"magnitude %.12f", got, want)
	}

	// And that per-pixel magnitude is brighter than the per-arcsecond one,
	// because the pixel is larger than an arcsecond.
	if want >= surface {
		t.Errorf("a 4 arcsec^2 pixel reads %.4f against %.4f per arcsec^2; more sky in a "+
			"pixel is more light, so the magnitude must be the smaller number", want, surface)
	}
}

// A hundredfold fainter threshold is exactly five magnitudes deeper.
//
// The definition of the magnitude scale, and the cheapest possible check that
// the 2.5 and the base-10 logarithm are the right way round — swapping either
// leaves a plausible, monotonic, wrong answer.
func TestLimitingMagnitudeUsesThePogsonRatio(t *testing.T) {
	t.Parallel()

	const (
		surface      = 21.5
		pixelArcsec2 = 1.0
		background   = 100.0
	)

	at1 := sbplan.LimitingMagnitude(surface, pixelArcsec2, background, background)
	at100 := sbplan.LimitingMagnitude(surface, pixelArcsec2, background, background/100)

	if diff := at100 - at1; math.Abs(diff-5) > 1e-12 {
		t.Errorf("a source a hundred times fainter is %.12f mag deeper, want exactly 5", diff)
	}
}

// A deeper threshold is a larger magnitude, monotonically.
func TestLimitingMagnitudeDeepensAsTheThresholdFalls(t *testing.T) {
	t.Parallel()

	prev := math.Inf(-1)

	for _, signal := range []float64{1000, 100, 10, 1, 0.1} {
		got := sbplan.LimitingMagnitude(21.5, 4, 50, signal)

		if got <= prev {
			t.Errorf("a threshold of %g e/s gives %.4f, no deeper than the %.4f of the "+
				"brighter one", signal, got, prev)
		}

		prev = got
	}
}

// A spec that cannot describe a measurement is refused at construction.
//
// Every one of these produces a finite, plausible magnitude if allowed
// through — a zero exposure divides, a zero aperture measures nothing, and a
// threshold of zero is reached by any source at all.
func TestNewImagingRefusesAnUnusableSpec(t *testing.T) {
	t.Parallel()

	site, err := coord.NewGeodetic(angle.Deg(-70.4045), angle.Deg(-24.6272), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	inst := optics.Instrument{
		Name:              "test",
		CollectingAreaM2:  0.03,
		PixelSolidAngleSR: 1e-11,
	}

	base := sbplan.Spec{
		Site:           site,
		Instrument:     inst,
		Exposure:       300 * gotime.Second,
		AperturePixels: 9,
		SNR:            5,
	}

	// Sky is nil in every case, and NewImaging checks it last on purpose, so
	// each case below must fail on its OWN field first. Asserting the message
	// is what makes that real: an earlier version of this test checked only
	// for ErrSpec, and since a nil Sky also returns ErrSpec, six of these
	// seven cases passed without ever reaching the field they were named for.
	for _, c := range []struct {
		name   string
		want   string
		mutate func(*sbplan.Spec)
	}{
		{"no sky", "dataset.Sky", func(*sbplan.Spec) {}},
		{"no site", "site", func(s *sbplan.Spec) { s.Site = nil }},
		{"zero exposure", "exposure", func(s *sbplan.Spec) { s.Exposure = 0 }},
		{"negative exposure", "exposure", func(s *sbplan.Spec) { s.Exposure = -gotime.Second }},
		{"zero aperture", "aperture", func(s *sbplan.Spec) { s.AperturePixels = 0 }},
		{"zero threshold", "threshold", func(s *sbplan.Spec) { s.SNR = 0 }},
		{"infinite threshold", "threshold", func(s *sbplan.Spec) { s.SNR = math.Inf(1) }},
		{"unusable instrument", "collecting area", func(s *sbplan.Spec) {
			s.Instrument = optics.Instrument{PixelSolidAngleSR: 1e-11}
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			spec := base
			c.mutate(&spec)

			_, err := sbplan.NewImaging(spec)
			if !errors.Is(err, sbplan.ErrSpec) {
				t.Fatalf("got %v, want ErrSpec", err)
			}

			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("the refusal is %q, which does not mention %q — it failed on some "+
					"other field than the one this case is about", err, c.want)
			}
		})
	}
}
