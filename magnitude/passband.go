package magnitude

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/unit"
)

// Sentinel errors for passband construction and projection. Match with
// errors.Is.
var (
	// ErrPassbandEmpty is returned when a passband has no response samples.
	ErrPassbandEmpty = errors.New("magnitude: passband has no response samples")

	// ErrPassbandResponse is returned when a response value is negative or
	// not finite, or when every response is zero.
	ErrPassbandResponse = errors.New("magnitude: passband response must be non-negative, finite, and not identically zero")

	// ErrPassbandCoverage is returned when a spectrum's grid does not cover
	// enough of a passband's response for the projection to be meaningful.
	ErrPassbandCoverage = errors.New("magnitude: spectral grid does not cover the passband")

	// ErrZeroPointUnknown is returned when a magnitude is requested in a
	// system the passband carries no zero point for — most often Vega,
	// which is reference-spectrum dependent and must be supplied.
	ErrZeroPointUnknown = errors.New("magnitude: passband has no zero point for this system")

	// ErrLuminousEfficacy is returned for an efficacy that cannot scale a
	// luminance. It belongs to the efficiency curve rather than to the
	// spectrum, so a zero here means the caller has not chosen one.
	ErrLuminousEfficacy = errors.New("magnitude: luminous efficacy must be positive and finite")
)

// System names a magnitude system. The systems differ in what defines
// magnitude zero, so a number is meaningless without one.
type System uint8

const (
	// AB is defined by a fixed flux density per unit frequency
	// (3631 Jy), needing no standard star — Oke & Gunn (1983).
	AB System = iota

	// Vega is defined by Vega itself having magnitude ~0 in every band. Its
	// zero point depends on which Vega reference spectrum is adopted
	// (Hayes, CALSPEC alpha_lyr_stis, ...), so it must travel with the
	// passband rather than being a constant of this package.
	Vega

	// ST is defined by a fixed flux density per unit wavelength
	// (3.631e-9 erg s^-1 cm^-2 A^-1).
	ST
)

// String renders the system name.
func (s System) String() string {
	switch s {
	case AB:
		return "AB"
	case Vega:
		return "Vega"
	case ST:
		return "ST"
	default:
		return fmt.Sprintf("System(%d)", uint8(s))
	}
}

// Detector distinguishes how a response curve weights photons, which
// changes the integral even for an identical curve shape.
type Detector uint8

const (
	// PhotonCounting responds to photon rate — CCDs, CMOS, photomultipliers.
	// The integrand carries an extra factor of lambda/hc.
	PhotonCounting Detector = iota

	// EnergyIntegrating responds to incident power — bolometers.
	EnergyIntegrating
)

// String renders the detector kind.
func (d Detector) String() string {
	if d == EnergyIntegrating {
		return "energy-integrating"
	}

	return "photon-counting"
}

// Passband is a named spectral response curve plus the metadata needed to
// turn a spectrum into a magnitude: which detector convention the curve
// assumes, and which magnitude systems it has zero points for.
//
// The response is dimensionless and need not be normalised — every
// projection divides by the curve's own integral, so an arbitrary scale
// cancels.
type Passband struct {
	// Name identifies the band, e.g. "Johnson V", "SDSS g", "SQM".
	Name string

	// WavelengthNM and Response are the tabulated curve, WavelengthNM
	// strictly increasing.
	WavelengthNM []unit.WavelengthNM
	Response     []float64

	// Detector states whether Response is a photon-counting or
	// energy-integrating response. Getting this wrong shifts a magnitude by
	// a band-dependent amount, so it is required rather than defaulted.
	Detector Detector

	// VegaZeroPointJy is the band-averaged flux density of Vega, in
	// janskys, for Vega-system magnitudes. Zero means "unknown", and a
	// Vega-system request then fails rather than silently substituting AB.
	VegaZeroPointJy float64

	// Reference cites the curve's origin, per the repository's provenance
	// convention.
	Reference string
}

// Validate reports whether the passband is usable.
func (p Passband) Validate() error {
	if len(p.WavelengthNM) == 0 || len(p.Response) == 0 {
		return fmt.Errorf("%w: %q", ErrPassbandEmpty, p.Name)
	}

	if len(p.WavelengthNM) != len(p.Response) {
		return fmt.Errorf("%w: %q has %d wavelengths and %d responses",
			ErrPassbandEmpty, p.Name, len(p.WavelengthNM), len(p.Response))
	}

	var total float64

	for i, r := range p.Response {
		if r < 0 || math.IsNaN(r) || math.IsInf(r, 0) {
			return fmt.Errorf("%w: %q sample %d = %g", ErrPassbandResponse, p.Name, i, r)
		}

		total += r

		if i > 0 && p.WavelengthNM[i] <= p.WavelengthNM[i-1] {
			return fmt.Errorf("%w: %q wavelengths not strictly increasing at %d",
				ErrPassbandEmpty, p.Name, i)
		}
	}

	if total == 0 {
		return fmt.Errorf("%w: %q is identically zero", ErrPassbandResponse, p.Name)
	}

	return nil
}

// Span reports the passband's wavelength range.
func (p Passband) Span() (lo, hi unit.WavelengthNM) {
	if len(p.WavelengthNM) == 0 {
		return 0, 0
	}

	return p.WavelengthNM[0], p.WavelengthNM[len(p.WavelengthNM)-1]
}

// Weights resamples the passband onto grid and returns the per-sample
// weights the projection integrals use, together with the fraction of the
// band's total response the grid actually covers.
//
// The weight already includes the photon-counting lambda factor when
// Detector is PhotonCounting, so callers integrate a plain product.
func (p Passband) Weights(grid unit.SpectralGrid) (weights []float64, coverage float64, err error) {
	if err := p.Validate(); err != nil {
		return nil, 0, err
	}

	if err := grid.Validate(); err != nil {
		return nil, 0, fmt.Errorf("magnitude: passband %q: %w", p.Name, err)
	}

	weights = make([]float64, grid.Len())
	if err := grid.Resample(weights, p.WavelengthNM, p.Response, 0); err != nil {
		return nil, 0, fmt.Errorf("magnitude: passband %q: %w", p.Name, err)
	}

	// Coverage compares the response the grid can see against the response
	// the curve actually has, on the curve's own axis. A grid that stops
	// inside the band silently truncates the integral, which is a wrong
	// answer rather than a slightly noisy one, so callers can refuse it.
	onGrid, err := grid.Integrate(weights)
	if err != nil {
		return nil, 0, fmt.Errorf("magnitude: passband %q: %w", p.Name, err)
	}

	full := trapezoid(p.WavelengthNM, p.Response)
	if full > 0 {
		coverage = onGrid / full
	}

	if p.Detector == PhotonCounting {
		for i := range weights {
			weights[i] *= float64(grid.At(i))
		}
	}

	return weights, coverage, nil
}

// MeanFluxDensity returns the passband-averaged spectral radiance of a
// spectrum sampled on grid, in the spectrum's own per-nanometre unit.
//
// This is the response-weighted mean the magnitude systems are defined
// against: the numerator integrates the spectrum against the band weights,
// the denominator normalises by the band, so an unnormalised response
// curve gives the same answer as a normalised one.
//
// minCoverage rejects a grid that covers less than that fraction of the
// band; pass 0 to accept any coverage.
func MeanFluxDensity(spectrum []float64, grid unit.SpectralGrid, p Passband, minCoverage float64) (float64, error) {
	weights, coverage, err := p.Weights(grid)
	if err != nil {
		return 0, err
	}

	if coverage < minCoverage {
		return 0, fmt.Errorf("%w: %q covered %.4f of the band, need %.4f",
			ErrPassbandCoverage, p.Name, coverage, minCoverage)
	}

	if len(spectrum) != grid.Len() {
		return 0, fmt.Errorf("%w: %d spectrum samples, grid has %d",
			unit.ErrGridMismatch, len(spectrum), grid.Len())
	}

	num := make([]float64, grid.Len())
	for i := range num {
		num[i] = spectrum[i] * weights[i]
	}

	numerator, err := grid.Integrate(num)
	if err != nil {
		return 0, fmt.Errorf("magnitude: passband %q numerator: %w", p.Name, err)
	}

	denominator, err := grid.Integrate(weights)
	if err != nil {
		return 0, fmt.Errorf("magnitude: passband %q normalisation: %w", p.Name, err)
	}

	if denominator == 0 {
		return 0, fmt.Errorf("%w: %q normalisation integral is zero", ErrPassbandResponse, p.Name)
	}

	return numerator / denominator, nil
}

// PivotWavelength returns the passband's pivot wavelength, the wavelength at
// which the per-wavelength and per-frequency flux densities of a source convert
// into one another exactly. It is the correct wavelength to use when moving a
// band-averaged f_lambda to f_nu, and unlike a mean or effective wavelength it
// depends only on the response curve.
//
// # It depends on the detector, and used not to
//
// The definition differs by a factor of lambda between the two conventions,
// because a photon-counting response is an energy response times lambda/hc:
//
//	photon counting: lambda_p^2 = INT(R lambda dl) / INT(R / lambda dl)
//	energy counting: lambda_p^2 = INT(R dl)        / INT(R / lambda^2 dl)
//
// This computed the photon-counting form for every band regardless of
// [Passband.Detector], which is wrong by up to nine tenths of a per cent in
// wavelength and twice that in an f_lambda to f_nu conversion - silent,
// systematic, and in the direction of making an energy-calibrated band look
// redder than it is.
//
// Checked against the Spanish Virtual Observatory's own WavelengthPivot for
// the five Bessell bands, all of which are energy counters: honouring the
// detector reproduces every one of them to four decimal places, where the
// photon form misses by 0.33 to 0.89 per cent.
func (p Passband) PivotWavelength() (unit.WavelengthNM, error) {
	if err := p.Validate(); err != nil {
		return 0, err
	}

	numer := make([]float64, len(p.Response))
	denom := make([]float64, len(p.Response))

	for i, r := range p.Response {
		lambda := float64(p.WavelengthNM[i])

		if p.Detector == EnergyIntegrating {
			numer[i], denom[i] = r, r/(lambda*lambda)
		} else {
			numer[i], denom[i] = r*lambda, r/lambda
		}
	}

	n := trapezoid(p.WavelengthNM, numer)
	d := trapezoid(p.WavelengthNM, denom)

	if d <= 0 {
		return 0, fmt.Errorf("%w: %q pivot denominator is zero", ErrPassbandResponse, p.Name)
	}

	return unit.WavelengthNM(math.Sqrt(n / d)), nil
}

// trapezoid integrates values tabulated at non-uniform wavelengths.
func trapezoid(x []unit.WavelengthNM, y []float64) float64 {
	if len(x) < 2 {
		return 0
	}

	var sum float64

	for i := 1; i < len(x); i++ {
		sum += 0.5 * (y[i] + y[i-1]) * float64(x[i]-x[i-1])
	}

	return sum
}

// jansky is 1e-26 W m^-2 Hz^-1, the flux-density unit the AB zero point is
// stated in.
const jansky = 1e-26

// SurfaceBrightness converts a spectral radiance, sampled on grid, into a
// surface brightness in mag/arcsec^2 in the given passband and magnitude
// system.
//
// The spectrum is W m^-2 sr^-1 nm^-1. The result is a magnitude per square
// arcsecond, which is a logarithmic quantity: never sum two of them, sum
// the radiances and convert once.
//
// minCoverage guards against a grid that only partially spans the band.
func SurfaceBrightness(
	spectrum []float64,
	grid unit.SpectralGrid,
	p Passband,
	sys System,
	minCoverage float64,
) (float64, error) {
	meanPerNM, err := MeanFluxDensity(spectrum, grid, p, minCoverage)
	if err != nil {
		return 0, err
	}

	if meanPerNM <= 0 {
		// A zero or negative band-averaged radiance has no magnitude. This
		// is a real state during Phase 0 (no components registered), so it
		// is reported as +Inf — arbitrarily faint — rather than an error or
		// a NaN that would propagate silently.
		return math.Inf(1), nil
	}

	// Per steradian to per square arcsecond.
	perArcsec2 := meanPerNM * constants.ArcsecondSquaredToSteradian

	switch sys {
	case AB:
		pivot, err := p.PivotWavelength()
		if err != nil {
			return 0, err
		}

		fNu := perWavelengthToPerFrequency(perArcsec2, pivot)
		zero := constants.Photometric.ABZeroPoint.Value

		return -2.5 * math.Log10(fNu/zero), nil

	case ST:
		// The ST zero point is defined per unit wavelength in
		// erg s^-1 cm^-2 A^-1; converted to SI per-nm that is 3.631e-9 *
		// 1e-7 J/erg * 1e4 cm^2/m^2 * 10 A/nm = 3.631e-11 W m^-2 nm^-1.
		const stZeroPointWattPerM2PerNM = 3.631e-11

		return -2.5 * math.Log10(perArcsec2/stZeroPointWattPerM2PerNM), nil

	case Vega:
		if p.VegaZeroPointJy <= 0 {
			return 0, fmt.Errorf("%w: %q has no Vega zero point", ErrZeroPointUnknown, p.Name)
		}

		pivot, err := p.PivotWavelength()
		if err != nil {
			return 0, err
		}

		fNu := perWavelengthToPerFrequency(perArcsec2, pivot)

		return -2.5 * math.Log10(fNu/(p.VegaZeroPointJy*jansky)), nil

	default:
		return 0, fmt.Errorf("%w: %v", ErrZeroPointUnknown, sys)
	}
}

// perWavelengthToPerFrequency converts a flux density per nanometre into
// one per hertz at the given wavelength, using f_nu = f_lambda * lambda^2/c.
func perWavelengthToPerFrequency(perNM float64, lambda unit.WavelengthNM) float64 {
	lambdaM := float64(lambda) * 1e-9
	perM := perNM * 1e9 // per nm -> per m

	return perM * lambdaM * lambdaM / constants.SI2019.SpeedOfLight.Value
}
