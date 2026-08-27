package optics

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/TuSKan/astrogo/unit"
)

// ErrExposure is returned for an exposure or aperture that cannot describe a
// measurement.
var ErrExposure = errors.New("optics: exposure and aperture must be positive and finite")

// SNR returns the signal-to-noise ratio of a source measured against a sky
// background, by the CCD equation of Merline & Howell (1995):
//
//	SNR = S*t / sqrt( S*t + n*(B + D)*t + n*R^2 )
//
// with S the source rate in electrons per second summed over the aperture, B
// the sky background in electrons per pixel per second, D the dark current, R
// the read noise in electrons per pixel, n the number of pixels in the
// measuring aperture and t the exposure.
//
// # Why the background is the interesting term
//
// Because it is the one a sky model can change. The other three are
// properties of the detector and the exposure, fixed once the equipment is
// chosen, while B is the whole reason a dark site is worth travelling to:
// under a bright sky the numerator is unchanged and the denominator grows,
// so the same source in the same instrument is detected or not depending on
// where and when it was pointed at. That is what makes this the natural
// meeting point between [github.com/TuSKan/astrogo/skybrightness] and an
// observing plan.
//
// # What a zero noise field means
//
// Exactly what it says: no read noise, or no dark current. Both are
// legitimate for a first estimate — a cooled sensor's dark current over a
// short exposure really is negligible — but neither is true of any real
// detector, and an SNR computed with both at zero is a background-limited
// upper bound rather than a prediction. Nothing here guesses a value for
// them, because a plausible guess would be indistinguishable in the result
// from a datasheet figure.
func (i Instrument) SNR(
	signal unit.ElectronsPerSecond,
	background unit.ElectronsPerPixelPerSecond,
	exposure time.Duration,
	pixels float64,
) (float64, error) {
	variance, err := i.noiseVariance(background, exposure, pixels)
	if err != nil {
		return 0, err
	}

	if signal < 0 || math.IsNaN(float64(signal)) {
		return 0, fmt.Errorf("%w: signal %g e/s", ErrExposure, float64(signal))
	}

	seconds := exposure.Seconds()
	collected := float64(signal) * seconds

	// The source contributes its own shot noise on top of everything the sky
	// and the detector contribute.
	total := collected + variance
	if total <= 0 {
		return 0, nil
	}

	return collected / math.Sqrt(total), nil
}

// LimitingSignal inverts [Instrument.SNR]: the faintest source rate that
// still reaches snr in one exposure of the given length against this
// background.
//
// # Why this rather than asking callers to search
//
// Because the inversion is closed-form and a search is not. Writing
// SNR = x/sqrt(x + N) with x the collected source electrons and N everything
// else in the variance gives a quadratic, x^2 - snr^2*x - snr^2*N = 0, whose
// positive root is
//
//	x = ( snr^2 + sqrt(snr^4 + 4*snr^2*N) ) / 2
//
// A caller who iterated [Instrument.SNR] would get the same answer more
// slowly and with a tolerance to choose, and this is the quantity an
// observing plan actually wants: the threshold, not the score.
//
// The result is a rate in electrons per second, deliberately not a magnitude.
// Turning it into one needs a passband and a zero point, which belong to
// [github.com/TuSKan/astrogo/magnitude]; this package knows about photons and
// electrons and has no business holding a photometric system.
func (i Instrument) LimitingSignal(
	background unit.ElectronsPerPixelPerSecond,
	exposure time.Duration,
	pixels, snr float64,
) (unit.ElectronsPerSecond, error) {
	variance, err := i.noiseVariance(background, exposure, pixels)
	if err != nil {
		return 0, err
	}

	if snr <= 0 || math.IsNaN(snr) || math.IsInf(snr, 0) {
		return 0, fmt.Errorf("%w: signal-to-noise ratio %g", ErrExposure, snr)
	}

	s2 := snr * snr
	collected := (s2 + math.Sqrt(s2*s2+4*s2*variance)) / 2

	return unit.ElectronsPerSecond(collected / exposure.Seconds()), nil
}

// noiseVariance is everything in the CCD equation's denominator except the
// source's own shot noise, in electrons squared.
//
// Shared so that [Instrument.SNR] and [Instrument.LimitingSignal] cannot
// drift apart: they are one equation and its inverse, and a term added to
// only one of them would make the round trip silently inconsistent.
func (i Instrument) noiseVariance(
	background unit.ElectronsPerPixelPerSecond, exposure time.Duration, pixels float64,
) (float64, error) {
	seconds := exposure.Seconds()

	if !positiveFinite(seconds) {
		return 0, fmt.Errorf("%w: exposure %v", ErrExposure, exposure)
	}

	if !positiveFinite(pixels) {
		return 0, fmt.Errorf("%w: aperture %g pixels", ErrExposure, pixels)
	}

	if background < 0 || math.IsNaN(float64(background)) {
		return 0, fmt.Errorf("%w: background %g e/pixel/s", ErrExposure, float64(background))
	}

	if i.ReadNoiseElectrons < 0 || i.DarkCurrentEPerSec < 0 {
		return 0, fmt.Errorf("%w: read noise %g e, dark current %g e/pixel/s",
			ErrExposure, i.ReadNoiseElectrons, i.DarkCurrentEPerSec)
	}

	// Sky and dark accumulate with time; read noise is charged once per
	// readout and enters as a variance, which is why it is squared here and
	// the other two are not.
	perPixel := (float64(background)+i.DarkCurrentEPerSec)*seconds +
		i.ReadNoiseElectrons*i.ReadNoiseElectrons

	return pixels * perPixel, nil
}
