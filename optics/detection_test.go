package optics_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/optics"
	"github.com/TuSKan/astrogo/unit"
)

// detectionInstrument is a plausible small CCD: 3.5 e read noise, 0.02
// e/pixel/s dark. The geometry does not matter here — SNR reads rates, not
// apertures — so it is set to something valid and ignored.
func detectionInstrument() optics.Instrument {
	return optics.Instrument{
		Name:               "test ccd",
		CollectingAreaM2:   0.03,
		PixelSolidAngleSR:  1e-11,
		ReadNoiseElectrons: 3.5,
		DarkCurrentEPerSec: 0.02,
	}
}

// The CCD equation, checked against a value worked by hand.
//
// # Why a hand-worked case rather than only invariants
//
// Because every invariant this file asserts — monotonicity, the sqrt(t) law,
// the round trip — is satisfied by a whole family of wrong formulas. A single
// arithmetic case pins which member of that family is implemented, and it is
// the only test here that would catch a transposed term.
//
//	S = 100 e/s, B = 20 e/pixel/s, D = 0.02, R = 3.5, n = 25 px, t = 60 s
//	source     = 100*60                       = 6000
//	per pixel  = (20 + 0.02)*60 + 3.5^2       = 1201.2 + 12.25 = 1213.45
//	variance   = 6000 + 25*1213.45            = 6000 + 30336.25 = 36336.25
//	SNR        = 6000 / sqrt(36336.25)        = 31.475...
func TestSNRMatchesAHandWorkedCase(t *testing.T) {
	t.Parallel()

	got, err := detectionInstrument().SNR(100, 20, 60*time.Second, 25)
	if err != nil {
		t.Fatalf("SNR: %v", err)
	}

	want := 6000 / math.Sqrt(6000+25*((20+0.02)*60+3.5*3.5))

	if math.Abs(got-want) > 1e-12 {
		t.Errorf("SNR = %.12f, want %.12f", got, want)
	}
}

// A brighter sky lowers the signal-to-noise ratio of the same source.
//
// This is the whole reason a sky-brightness model reaches an observing plan
// at all: the numerator does not move, the denominator grows, and the same
// target in the same instrument becomes undetectable somewhere brighter.
func TestSNRFallsWithABrighterSky(t *testing.T) {
	t.Parallel()

	inst := detectionInstrument()
	prev := math.Inf(1)

	for _, background := range []unit.ElectronsPerPixelPerSecond{1, 10, 100, 1000} {
		got, err := inst.SNR(100, background, 60*time.Second, 25)
		if err != nil {
			t.Fatalf("SNR at %g e/pixel/s: %v", float64(background), err)
		}

		if got >= prev {
			t.Errorf("a sky of %g e/pixel/s gives SNR %.4f, no worse than the %.4f of the "+
				"darker one", float64(background), got, prev)
		}

		prev = got
	}
}

// In the background-limited regime the signal-to-noise ratio grows as the
// square root of exposure.
//
// The classic result, and it holds only where the sky dominates read noise —
// which is why the background here is large and the exposures long. It is
// worth asserting because it is the property that fails first if read noise
// is charged per second rather than per readout.
func TestSNRGrowsAsSqrtExposureWhenSkyLimited(t *testing.T) {
	t.Parallel()

	inst := detectionInstrument()

	short, err := inst.SNR(1, 500, 100*time.Second, 25)
	if err != nil {
		t.Fatalf("SNR: %v", err)
	}

	long, err := inst.SNR(1, 500, 400*time.Second, 25)
	if err != nil {
		t.Fatalf("SNR: %v", err)
	}

	// Four times the exposure, twice the SNR, to within the small correction
	// the source's own shot noise and the read term still contribute.
	if ratio := long / short; math.Abs(ratio-2) > 0.01 {
		t.Errorf("quadrupling the exposure changed SNR by %.4f×, want 2× — read noise must "+
			"be charged once per readout rather than accumulating with time", ratio)
	}
}

// LimitingSignal is the exact inverse of SNR.
//
// Round-tripping is the strongest statement available about the pair: the
// closed-form root and the forward equation are written separately, so
// agreeing to floating-point noise across four orders of magnitude of sky
// means neither drifted from the other.
func TestLimitingSignalInvertsSNR(t *testing.T) {
	t.Parallel()

	inst := detectionInstrument()

	for _, background := range []unit.ElectronsPerPixelPerSecond{0, 1, 50, 5000} {
		for _, snr := range []float64{3, 5, 10, 100} {
			signal, err := inst.LimitingSignal(background, 300*time.Second, 9, snr)
			if err != nil {
				t.Fatalf("LimitingSignal: %v", err)
			}

			back, err := inst.SNR(signal, background, 300*time.Second, 9)
			if err != nil {
				t.Fatalf("SNR: %v", err)
			}

			if math.Abs(back-snr)/snr > 1e-12 {
				t.Errorf("background %g, snr %g: the limiting signal %g e/s reads back as "+
					"SNR %.12f", float64(background), snr, float64(signal), back)
			}
		}
	}
}

// A brighter sky demands a brighter source to reach the same threshold.
func TestLimitingSignalRisesWithTheSky(t *testing.T) {
	t.Parallel()

	inst := detectionInstrument()
	prev := 0.0

	for _, background := range []unit.ElectronsPerPixelPerSecond{1, 10, 100, 1000} {
		got, err := inst.LimitingSignal(background, 60*time.Second, 25, 5)
		if err != nil {
			t.Fatalf("LimitingSignal: %v", err)
		}

		if float64(got) <= prev {
			t.Errorf("a sky of %g e/pixel/s needs only %g e/s, no more than the %g of the "+
				"darker one", float64(background), float64(got), prev)
		}

		prev = float64(got)
	}
}

// Inputs that cannot describe a measurement are refused rather than returning
// a plausible number.
//
// A zero exposure divides; a zero aperture measures nothing; a negative
// background or read noise is a sign error somewhere upstream. All four
// produce a finite, innocent-looking float if left alone.
func TestDetectionRefusesImpossibleInputs(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name       string
		inst       optics.Instrument
		background unit.ElectronsPerPixelPerSecond
		exposure   time.Duration
		pixels     float64
	}{
		{"zero exposure", detectionInstrument(), 10, 0, 25},
		{"negative exposure", detectionInstrument(), 10, -time.Second, 25},
		{"zero aperture", detectionInstrument(), 10, time.Second, 0},
		{"negative background", detectionInstrument(), -1, time.Second, 25},
		{"negative read noise", optics.Instrument{
			CollectingAreaM2: 0.03, PixelSolidAngleSR: 1e-11, ReadNoiseElectrons: -1,
		}, 10, time.Second, 25},
		{"negative dark current", optics.Instrument{
			CollectingAreaM2: 0.03, PixelSolidAngleSR: 1e-11, DarkCurrentEPerSec: -1,
		}, 10, time.Second, 25},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if _, err := c.inst.SNR(100, c.background, c.exposure, c.pixels); !errors.Is(err, optics.ErrExposure) {
				t.Errorf("SNR accepted it: %v", err)
			}

			if _, err := c.inst.LimitingSignal(c.background, c.exposure, c.pixels, 5); !errors.Is(err, optics.ErrExposure) {
				t.Errorf("LimitingSignal accepted it: %v", err)
			}
		})
	}
}

// A non-positive signal-to-noise threshold is refused.
//
// Separate from the table above because it applies only to the inversion:
// SNR itself is happy to report that a source is undetectable, but there is
// no source faint enough to "reach" a threshold of zero.
func TestLimitingSignalRefusesANonPositiveThreshold(t *testing.T) {
	t.Parallel()

	inst := detectionInstrument()

	for _, snr := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, err := inst.LimitingSignal(10, time.Second, 25, snr); !errors.Is(err, optics.ErrExposure) {
			t.Errorf("a threshold of %g was accepted: %v", snr, err)
		}
	}
}
