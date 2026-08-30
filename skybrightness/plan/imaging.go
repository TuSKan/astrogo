package plan

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/optics"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/skybrightness/dataset"
	"github.com/TuSKan/astrogo/time"
)

// ErrSpec is returned for a request this package cannot honestly answer.
var ErrSpec = errors.New("skybrightness/plan: unusable request")

// Spec describes an imaging setup completely enough to say how faint it can
// see.
//
// Every field is required and none has a default, which is the point: a
// limiting magnitude is a threshold, and each of these moves it. A package
// that guessed an exposure or a detection threshold would be answering a
// different question from the one asked.
type Spec struct {
	// Sky is the assembled sky-brightness model.
	Sky *dataset.Sky

	// Site is where the instrument stands.
	Site *coord.Geodetic

	// Air is the atmosphere to evaluate under, as [dataset.Sky.Scene] takes
	// it. Nil means a clear atmosphere at the site's own elevation.
	Air *atmosphere.Builder

	// Instrument is the radiometric chain, including its noise terms.
	Instrument optics.Instrument

	// Exposure is one integration.
	Exposure time.Duration

	// AperturePixels is how many pixels the measurement sums over — the
	// photometric aperture, not the whole sensor.
	AperturePixels float64

	// SNR is the signal-to-noise ratio that counts as a detection. Five is
	// the usual convention for a confident one.
	SNR float64
}

// Imaging reports how faint an imaging system can see through a modelled sky.
//
// It satisfies [plan.SkyDepth], so it drops straight into
// [plan.LimitingMagnitudeConstraint].
//
// A value is safe for concurrent use: evaluation performs no I/O and holds no
// mutable state.
type Imaging struct {
	spec         Spec
	pixelArcsec2 float64
	bandName     string
}

// Compile-time assertion that this is what the planning engine asked for.
// The interface is one method, so a drift is unlikely — but it is the whole
// contract between two packages that otherwise share nothing.
var _ plan.SkyDepth = (*Imaging)(nil)

// NewImaging validates a spec and returns the depth model for it.
func NewImaging(spec Spec) (*Imaging, error) {
	// Sky is checked last, not first. Every other field can be validated
	// without one, and a caller who got two things wrong is better served by
	// hearing about the one they can see in their own code than about the
	// object they were about to pass in.
	switch {
	case spec.Site == nil:
		return nil, fmt.Errorf("%w: needs a site", ErrSpec)
	case spec.Exposure <= 0:
		return nil, fmt.Errorf("%w: exposure %v", ErrSpec, spec.Exposure)
	case !(spec.AperturePixels > 0) || math.IsInf(spec.AperturePixels, 0):
		return nil, fmt.Errorf("%w: aperture %g pixels", ErrSpec, spec.AperturePixels)
	case !(spec.SNR > 0) || math.IsInf(spec.SNR, 0):
		return nil, fmt.Errorf("%w: detection threshold %g", ErrSpec, spec.SNR)
	}

	if err := spec.Instrument.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSpec, err)
	}

	if spec.Sky == nil {
		return nil, fmt.Errorf("%w: needs an assembled dataset.Sky", ErrSpec)
	}

	// A pixel's solid angle in square arcseconds, which is the unit it has
	// to be in to sit beside a surface brightness per square arcsecond.
	// constants already carries the conversion; restating it here would be a
	// second copy free to disagree with the one the estimate used.
	return &Imaging{
		spec:         spec,
		pixelArcsec2: spec.Instrument.PixelSolidAngleSR / constants.ArcsecondSquaredToSteradian,
		bandName:     spec.Sky.Band().Name,
	}, nil
}

// LimitingMagnitudeAt implements [plan.SkyDepth].
//
// # How a magnitude comes out of an electron rate without a zero point
//
// It does not need one, because the sky supplies it. The same estimate yields
// both a surface brightness in mag/arcsec² and a detector background in
// electrons per pixel per second, through the same instrument and from the
// same stored spectrum. That pair *is* a photometric calibration: a pixel of
// solid angle Ω sees a source of magnitude
//
//	m_pixel = SB - 2.5*log10(Ω)
//
// and it produces B electrons per second, so anything producing S electrons
// per second is at
//
//	m = m_pixel + 2.5*log10(B/S)
//
// which is the whole conversion. Nothing here holds a zero point, an
// effective wavelength or a system response — they would all have to agree
// with the ones the estimate already used, and a second copy that can
// disagree is worse than no copy.
//
// # The assumption this makes, stated plainly
//
// That the source has the same colour as the sky. The electrons-to-magnitude
// step depends on spectral shape, and the calibration above is anchored on
// the sky's shape. For a red source under a moonlit sky, or a blue one under
// sodium skyglow, the answer is off by the colour term between them. Getting
// that right needs the source's own spectrum, which a target list does not
// carry; what this returns is the depth reached for a sky-coloured source,
// which is the same convention a sky-limited exposure-time calculator uses.
//
// # Put the filter in the throughput, or the band label is a half-truth
//
// The two halves of the calibration are measured over different wavelength
// ranges. [optics.Instrument.BackgroundRate] integrates the whole spectral
// grid weighted by the instrument's own Throughput, while the surface
// brightness is measured through the sky model's passband. Those coincide
// only when that filter is one of the Throughput elements.
//
// An instrument declaring no throughput at all is treated as perfectly
// transmitting at every wavelength, so its electrons come from the entire
// grid while its magnitudes are quoted in the model's band. The result stays
// internally consistent — a sky-coloured source really would produce that
// rate — but it is not a filter magnitude, and it does not respond to airmass
// the way one would: extinction is wavelength-dependent, so a broadband rate
// and a V-band brightness diverge as a target sinks. Measured at Paranal, a
// filterless setup lost only 0.09 magnitudes of depth between the zenith and
// ten degrees where the V-band sky brightened by 0.69.
//
// So declare the filter. [Band] reports which one the magnitudes are on, and
// the answer is fully meaningful only when the instrument passes that band
// and little else.
func (i *Imaging) LimitingMagnitudeAt(t time.Time, alt, az angle.Angle) (float64, error) {
	scene, err := i.spec.Sky.Scene(i.spec.Site, t.GoTime(), i.spec.Air)
	if err != nil {
		return 0, fmt.Errorf("skybrightness/plan: scene: %w", err)
	}

	// Evaluation does no I/O, so the context only bounds a CPU-bound
	// integral, and one direction is milliseconds even at Reference fidelity.
	// plan's Constraint carries no context to thread through, so there is
	// nothing here to cancel with.
	est, err := i.spec.Sky.Direction(context.Background(), scene, alt, az)
	if err != nil {
		return 0, fmt.Errorf("skybrightness/plan: estimate: %w", err)
	}

	surface, err := i.spec.Sky.SurfaceBrightness(est)
	if err != nil {
		return 0, fmt.Errorf("skybrightness/plan: surface brightness: %w", err)
	}

	background, err := est.ElectronRate(i.spec.Instrument)
	if err != nil {
		return 0, fmt.Errorf("skybrightness/plan: background rate: %w", err)
	}

	signal, err := i.spec.Instrument.LimitingSignal(
		background, i.spec.Exposure, i.spec.AperturePixels, i.spec.SNR)
	if err != nil {
		return 0, fmt.Errorf("skybrightness/plan: limiting signal: %w", err)
	}

	if background <= 0 || signal <= 0 {
		return 0, fmt.Errorf("%w: the sky produces %g e/pixel/s and the threshold %g e/s, "+
			"which cannot be turned into a magnitude",
			ErrSpec, float64(background), float64(signal))
	}

	return limitingMagnitude(surface, i.pixelArcsec2,
		float64(background), float64(signal)), nil
}

// limitingMagnitude is the conversion described above, split out so it can be
// checked against worked arithmetic without a radiance engine, a network and
// 145 MB of reference data standing between the test and the four numbers it
// is actually about.
func limitingMagnitude(surfaceMag, pixelArcsec2, background, signal float64) float64 {
	// What one pixel's worth of sky would be as a point source.
	pixelMag := surfaceMag - 2.5*math.Log10(pixelArcsec2)

	// That pixel produces `background` electrons per second, so `signal`
	// electrons per second is this much fainter.
	return pixelMag + 2.5*math.Log10(background/signal)
}

// Band names the passband the returned magnitudes are quoted on.
//
// Worth asking for rather than assuming, for two reasons. It is the sky
// model's own band, so comparing the result against a catalogue magnitude in
// a different filter is a mistake that produces entirely plausible numbers.
// And it describes the magnitude *scale* rather than the wavelengths the
// electrons came from — those are the instrument's Throughput, and the two
// agree only if this filter is in it. See [Imaging.LimitingMagnitudeAt].
func (i *Imaging) Band() string { return i.bandName }
