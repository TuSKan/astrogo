package skybrightness

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/unit"
)

// PassbandID names one passband/response curve.
type PassbandID string

// MagSystem names a photometric magnitude system. A magnitude value
// without a system attached is exactly the ambiguity this package forbids
// — every SurfaceBrightnessAB/SurfaceBrightnessVega is always paired with
// the Passband (and hence the MagSystem) that produced it.
type MagSystem uint8

// The three magnitude-system kinds.
const (
	SystemAB MagSystem = iota
	SystemVega
	// SystemPhotometricNone marks a response curve that is not a
	// magnitude-system passband at all (CIE V(lambda), an SQM device
	// response) — its integral is a derived quantity (luminance, an
	// instrument reading), not a magnitude.
	SystemPhotometricNone
)

// String implements fmt.Stringer.
func (s MagSystem) String() string {
	switch s {
	case SystemAB:
		return "AB"
	case SystemVega:
		return "Vega"
	case SystemPhotometricNone:
		return "PhotometricNone"
	default:
		return "MagSystem(unknown)"
	}
}

// DetectorType selects whether a passband's integration weights by photon
// count (PhotonCounting — CCD/CMOS) or by energy (EnergyIntegrating —
// bolometric).
type DetectorType uint8

// The two detector-weighting kinds.
const (
	PhotonCounting DetectorType = iota
	EnergyIntegrating
)

// VegaZeroPoint is a passband's Vega-system calibration: Vega's own
// band-mean spectral radiance in this band, which SurfaceBrightnessVega
// values in this passband are magnitudes relative to.
type VegaZeroPoint struct {
	MeanFlambda unit.SpectralRadiance // Vega's band-mean spectral radiance in this band
	Spectrum    string                // e.g. "CALSPEC alpha_lyr_stis_011"
	Uncertainty float64               // mag
}

// ErrInvalidPassband is returned by Passband.Validate.
var ErrInvalidPassband = errors.New("skybrightness: invalid passband")

// ErrPassbandNotFound is returned by PassbandSet.Get for an unknown ID.
var ErrPassbandNotFound = errors.New("skybrightness: passband not found")

// PassbandSet is a versioned collection of passbands. No implementation in
// this package tabulates response curves in Go source — see
// skybrightness/dataset/passband for the real, checksummed providers
// (docs/skybrightness.md §3).
type PassbandSet interface {
	Get(id PassbandID) (*Passband, error)
	List() []PassbandID
	Version() DatasetVersion
}

type staticPassbandSet struct {
	version DatasetVersion
	byID    map[PassbandID]*Passband
	order   []PassbandID
}

// NewPassbandSet builds a PassbandSet from caller-supplied passbands — a
// pure, in-memory implementation with no I/O, suitable for tests, the
// fast top-hat analytic passbands, or a caller that already has its
// curves in hand.
func NewPassbandSet(version DatasetVersion, pbs ...*Passband) PassbandSet {
	s := &staticPassbandSet{version: version, byID: make(map[PassbandID]*Passband, len(pbs))}

	for _, p := range pbs {
		if _, exists := s.byID[p.ID]; !exists {
			s.order = append(s.order, p.ID)
		}

		s.byID[p.ID] = p
	}

	return s
}

func (s *staticPassbandSet) Get(id PassbandID) (*Passband, error) {
	p, ok := s.byID[id]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrPassbandNotFound, id)
	}

	return p, nil
}

func (s *staticPassbandSet) List() []PassbandID {
	cp := make([]PassbandID, len(s.order))
	copy(cp, s.order)

	return cp
}

func (s *staticPassbandSet) Version() DatasetVersion { return s.version }

// Passband is a response curve: a wavelength grid and a dimensionless,
// non-negative response at each point. No passband response curves are
// tabulated in this package's source — see
// skybrightness/dataset/passband for the real, versioned, checksummed
// data providers (docs/skybrightness.md §3); TopHat/Gaussian below exist
// only for tests and quick analytic use.
type Passband struct {
	ID         PassbandID
	System     MagSystem
	Detector   DetectorType
	Wavelength []unit.WavelengthNM // strictly increasing
	Response   []float64           // dimensionless, >= 0
	VegaZP     *VegaZeroPoint      // nil unless calibrated
	Version    DatasetVersion
	Source     SourceRef
}

// Validate checks the passband's internal consistency.
func (p *Passband) Validate() error {
	if len(p.Wavelength) < 2 || len(p.Wavelength) != len(p.Response) {
		return fmt.Errorf("%w: Wavelength/Response must have >= 2 equal-length entries (got %d, %d)",
			ErrInvalidPassband, len(p.Wavelength), len(p.Response))
	}

	anyPositive := false

	for i, r := range p.Response {
		if r < 0 {
			return fmt.Errorf("%w: Response[%d] = %g, must be >= 0", ErrInvalidPassband, i, r)
		}

		if r > 0 {
			anyPositive = true
		}

		if i > 0 && p.Wavelength[i] <= p.Wavelength[i-1] {
			return fmt.Errorf("%w: Wavelength must be strictly increasing (index %d)", ErrInvalidPassband, i)
		}
	}

	if !anyPositive {
		return fmt.Errorf("%w: Response is identically zero", ErrInvalidPassband)
	}

	return nil
}

// Range returns the passband's native wavelength range.
func (p *Passband) Range() (lo, hi unit.WavelengthNM) {
	return p.Wavelength[0], p.Wavelength[len(p.Wavelength)-1]
}

// PivotWavelength returns the passband's pivot wavelength:
// lambda_p^2 = integral(R*lambda dlambda) / integral(R/lambda dlambda).
// The pivot wavelength is the point at which photon-flux and energy-flux
// calibration agree, and is the conventional reference wavelength for
// ABSurfaceBrightness.
func (p *Passband) PivotWavelength() unit.WavelengthNM {
	num, den := 0.0, 0.0

	for i := range p.Wavelength {
		w := nativeWeight(p.Wavelength, i)
		lam := float64(p.Wavelength[i])
		num += p.Response[i] * lam * w
		den += p.Response[i] / lam * w
	}

	if den <= 0 {
		return 0
	}

	return unit.WavelengthNM(math.Sqrt(num / den))
}

// EffectiveWavelength returns the passband's response-weighted mean
// wavelength: integral(R*lambda dlambda) / integral(R dlambda). A simpler,
// deliberately different quantity from PivotWavelength (which accounts for
// photon-vs-energy weighting) — see docs/skybrightness.md §3.
func (p *Passband) EffectiveWavelength() unit.WavelengthNM {
	num, den := 0.0, 0.0

	for i := range p.Wavelength {
		w := nativeWeight(p.Wavelength, i)
		num += p.Response[i] * float64(p.Wavelength[i]) * w
		den += p.Response[i] * w
	}

	if den <= 0 {
		return 0
	}

	return unit.WavelengthNM(num / den)
}

func nativeWeight(lambda []unit.WavelengthNM, i int) float64 {
	n := len(lambda)

	switch {
	case n == 1:
		return 1
	case i == 0:
		return float64(lambda[1]-lambda[0]) / 2
	case i == n-1:
		return float64(lambda[n-1]-lambda[n-2]) / 2
	default:
		return float64(lambda[i+1]-lambda[i-1]) / 2
	}
}

// TopHat returns an analytic rectangular passband, Response == 1 across
// [lo, hi] and 0 outside — for tests and quick analytic use, never a
// substitute for a real response curve.
func TopHat(id PassbandID, lo, hi unit.WavelengthNM) *Passband {
	return &Passband{
		ID: id, System: SystemPhotometricNone, Detector: PhotonCounting,
		Wavelength: []unit.WavelengthNM{lo, hi}, Response: []float64{1, 1},
		Source: SourceRef{Name: "skybrightness.TopHat (analytic, test-grade)", Fidelity: FidelitySynthetic},
	}
}

// Gaussian returns an analytic Gaussian passband centered at center with
// the given full width at half maximum — for tests and quick analytic
// use, never a substitute for a real response curve.
func Gaussian(id PassbandID, center, fwhm unit.WavelengthNM) *Passband {
	sigma := float64(fwhm) / 2.3548200450309493 // 2*sqrt(2*ln2)
	n := 41
	lo := float64(center) - 3*sigma
	step := 6 * sigma / float64(n-1)

	wl := make([]unit.WavelengthNM, n)
	resp := make([]float64, n)

	for i := range n {
		lam := lo + float64(i)*step
		wl[i] = unit.WavelengthNM(lam)
		d := (lam - float64(center)) / sigma
		resp[i] = math.Exp(-0.5 * d * d)
	}

	return &Passband{
		ID: id, System: SystemPhotometricNone, Detector: PhotonCounting,
		Wavelength: wl, Response: resp,
		Source: SourceRef{Name: "skybrightness.Gaussian (analytic, test-grade)", Fidelity: FidelitySynthetic},
	}
}

// MinPassbandCoverage is the minimum fraction of a passband's own native
// response integral that a SpectralGrid must cover before an integration
// function accepts it. Below this, IntegrateRadiance/IntegratePhotonRadiance/
// BandMeanSpectralRadiance return ErrPassbandCoverageTooLow rather than
// silently truncating.
const MinPassbandCoverage = 0.99

// ErrPassbandCoverageTooLow is returned when a SpectralGrid covers less
// than MinPassbandCoverage of a passband's native response integral.
var ErrPassbandCoverageTooLow = errors.New("skybrightness: spectral grid covers too little of the passband response")

// resampleResponse linearly interpolates p.Response onto g's wavelengths
// (0 outside p's native range) and errors if g covers less than
// MinPassbandCoverage of p's own native response integral.
func resampleResponse(g SpectralGrid, p *Passband) (resp []float64, err error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}

	lambda := g.Lambda()
	resp = make([]float64, len(lambda))

	for i, lam := range lambda {
		resp[i] = interpResponse(p, lam)
	}

	usedIntegral := trapezoidDot(resp, g.Weights())
	nativeIntegral := trapezoidDot(p.Response, nativeWeights(p.Wavelength))

	if nativeIntegral <= 0 {
		return resp, fmt.Errorf("%w: passband %q has zero native response integral", ErrInvalidPassband, p.ID)
	}

	coverage := usedIntegral / nativeIntegral
	if coverage > 1 {
		coverage = 1 // grid can extend beyond p's native range; that's full coverage, not >100%
	}

	if coverage < MinPassbandCoverage {
		return resp, fmt.Errorf("%w: passband %q, grid covers %.1f%% (need >= %.0f%%)",
			ErrPassbandCoverageTooLow, p.ID, coverage*100, MinPassbandCoverage*100)
	}

	return resp, nil
}

func interpResponse(p *Passband, lam unit.WavelengthNM) float64 {
	wl := p.Wavelength
	if lam < wl[0] || lam > wl[len(wl)-1] {
		return 0
	}

	// binary search for the bracketing interval
	lo, hi := 0, len(wl)-1

	for hi-lo > 1 {
		mid := (lo + hi) / 2
		if wl[mid] <= lam {
			lo = mid
		} else {
			hi = mid
		}
	}

	if wl[hi] == wl[lo] {
		return p.Response[lo]
	}

	frac := float64(lam-wl[lo]) / float64(wl[hi]-wl[lo])

	return p.Response[lo] + frac*(p.Response[hi]-p.Response[lo])
}

func nativeWeights(lambda []unit.WavelengthNM) []float64 {
	w := make([]float64, len(lambda))
	for i := range lambda {
		w[i] = nativeWeight(lambda, i)
	}

	return w
}

func trapezoidDot(values, weights []float64) float64 {
	sum := 0.0
	for i, v := range values {
		sum += v * weights[i]
	}

	return sum
}

// equivalentWidth returns integral(R dlambda) / max(R) — the rectangular
// width whose area under a flat response == the passband's real response
// integral. IntegrateRadiance/IntegratePhotonRadiance use it to turn a
// response-weighted mean spectral radiance into a passband-integrated
// radiance in self-consistent units (docs/skybrightness.md §3 states this
// is Phase 1's chosen, documented convention).
func equivalentWidth(resp []float64, weights []float64) float64 {
	maxR := 0.0
	for _, r := range resp {
		if r > maxR {
			maxR = r
		}
	}

	if maxR <= 0 {
		return 0
	}

	return trapezoidDot(resp, weights) / maxR
}

// BandMeanSpectralRadiance returns the response-weighted mean spectral
// radiance over the passband: integral(L*R dlambda) / integral(R dlambda).
func BandMeanSpectralRadiance(g SpectralGrid, spec []unit.SpectralRadiance, p *Passband) (unit.SpectralRadiance, error) {
	if len(spec) != g.Len() {
		return 0, fmt.Errorf("%w: spec length %d != grid length %d", ErrInvalidPassband, len(spec), g.Len())
	}

	resp, err := resampleResponse(g, p)
	if err != nil {
		return 0, err
	}

	w := g.Weights()

	num, den := 0.0, 0.0
	for i, l := range spec {
		num += float64(l) * resp[i] * w[i]
		den += resp[i] * w[i]
	}

	if den <= 0 {
		return 0, fmt.Errorf("%w: zero response integral on grid", ErrInvalidPassband)
	}

	return unit.SpectralRadiance(num / den), nil
}

// IntegrateRadiance returns the passband-integrated radiance:
// BandMeanSpectralRadiance x equivalent width (docs/skybrightness.md §3).
func IntegrateRadiance(g SpectralGrid, spec []unit.SpectralRadiance, p *Passband) (unit.Radiance, error) {
	if len(spec) != g.Len() {
		return 0, fmt.Errorf("%w: spec length %d != grid length %d", ErrInvalidPassband, len(spec), g.Len())
	}

	resp, err := resampleResponse(g, p)
	if err != nil {
		return 0, err
	}

	w := g.Weights()

	num, den := 0.0, 0.0
	for i, l := range spec {
		num += float64(l) * resp[i] * w[i]
		den += resp[i] * w[i]
	}

	if den <= 0 {
		return 0, fmt.Errorf("%w: zero response integral on grid", ErrInvalidPassband)
	}

	mean := num / den

	return unit.Radiance(mean * equivalentWidth(resp, w)), nil
}

// IntegratePhotonRadiance is IntegrateRadiance's photon-flux analogue: spec
// is first converted per-wavelength via constants.ToPhoton.
func IntegratePhotonRadiance(g SpectralGrid, spec []unit.SpectralRadiance, p *Passband) (unit.PhotonRadiance, error) {
	if len(spec) != g.Len() {
		return 0, fmt.Errorf("%w: spec length %d != grid length %d", ErrInvalidPassband, len(spec), g.Len())
	}

	resp, err := resampleResponse(g, p)
	if err != nil {
		return 0, err
	}

	lambda := g.Lambda()
	w := g.Weights()

	num, den := 0.0, 0.0

	for i, l := range spec {
		photon := float64(constants.ToPhoton(l, lambda[i]))
		num += photon * resp[i] * w[i]
		den += resp[i] * w[i]
	}

	if den <= 0 {
		return 0, fmt.Errorf("%w: zero response integral on grid", ErrInvalidPassband)
	}

	mean := num / den

	return unit.PhotonRadiance(mean * equivalentWidth(resp, w)), nil
}

// ABSurfaceBrightness converts a band-mean spectral radiance (at the given
// reference wavelength, conventionally the passband's PivotWavelength)
// into an AB-system surface brightness, mag/arcsec^2:
//
//	f_nu = f_lambda * lambda^2 / c            (per steradian, from mean's own /sr)
//	f_nu,arcsec2 = f_nu * (1 arcsec)^2 in steradians
//	m_AB = -2.5*log10(f_nu,arcsec2 / 3631 Jy)
//
// Returns +Inf for mean <= 0 (a measured zero, not an error — see
// docs/skybrightness.md §1's forbidden-shortcuts discussion of the
// analogous VIIRS zero case).
func ABSurfaceBrightness(mean unit.SpectralRadiance, pivot unit.WavelengthNM) unit.SurfaceBrightnessAB {
	if mean <= 0 || pivot <= 0 {
		return unit.SurfaceBrightnessAB(math.Inf(1))
	}

	fNuPerArcsec2 := fNuPerArcsec2FromFLambda(mean, pivot)

	return unit.SurfaceBrightnessAB(-2.5 * math.Log10(fNuPerArcsec2/constants.Photometric.ABZeroPoint.Value))
}

// ABToBandMean is the round-trip inverse of ABSurfaceBrightness.
func ABToBandMean(ab unit.SurfaceBrightnessAB, pivot unit.WavelengthNM) unit.SpectralRadiance {
	if pivot <= 0 {
		return 0
	}

	fNuPerArcsec2 := constants.Photometric.ABZeroPoint.Value * math.Pow(10, -0.4*float64(ab))

	return fLambdaFromFNuPerArcsec2(fNuPerArcsec2, pivot)
}

func fNuPerArcsec2FromFLambda(mean unit.SpectralRadiance, lambda unit.WavelengthNM) float64 {
	lambdaM := float64(lambda) * 1e-9
	fLambdaPerM := float64(mean) * 1e9 // W/m^2/sr/nm -> W/m^2/sr/m
	fNuPerSr := fLambdaPerM * lambdaM * lambdaM / constants.SI2019.SpeedOfLight.Value

	return fNuPerSr * constants.ArcsecondSquaredToSteradian
}

func fLambdaFromFNuPerArcsec2(fNuPerArcsec2 float64, lambda unit.WavelengthNM) unit.SpectralRadiance {
	lambdaM := float64(lambda) * 1e-9
	fNuPerSr := fNuPerArcsec2 / constants.ArcsecondSquaredToSteradian
	fLambdaPerM := fNuPerSr * constants.SI2019.SpeedOfLight.Value / (lambdaM * lambdaM)

	return unit.SpectralRadiance(fLambdaPerM * 1e-9)
}

// ErrNoVegaZeroPoint is returned by VegaSurfaceBrightness for a passband
// with no VegaZP.
var ErrNoVegaZeroPoint = errors.New("skybrightness: passband has no Vega zero point")

// VegaSurfaceBrightness converts a band-mean spectral radiance into a
// Vega-system surface brightness, mag/arcsec^2, relative to p.VegaZP.
func VegaSurfaceBrightness(mean unit.SpectralRadiance, p *Passband) (unit.SurfaceBrightnessVega, error) {
	if p.VegaZP == nil {
		return 0, fmt.Errorf("%w: passband %q", ErrNoVegaZeroPoint, p.ID)
	}

	if mean <= 0 {
		return unit.SurfaceBrightnessVega(math.Inf(1)), nil
	}

	return unit.SurfaceBrightnessVega(-2.5 * math.Log10(float64(mean)/float64(p.VegaZP.MeanFlambda))), nil
}

// cieLuminousEfficacyLmPerW is the defined photopic luminous efficacy at
// 555 nm, 683 lm/W — exact by CIE/SI photometric convention since 1979
// (the value candela's SI definition is built around).
const cieLuminousEfficacyLmPerW = 683.0

// cieScotopicPeakEfficacyLmPerW is the conventional scotopic luminous
// efficacy at 507 nm, 1700 lm/W (CIE 1951 scotopic V'(lambda)).
const cieScotopicPeakEfficacyLmPerW = 1700.0

// PhotopicLuminance returns the CIE photopic luminance of spec, weighted
// by vlambda (the CIE V(lambda) response, System == SystemPhotometricNone):
// L_v = 683 lm/W * integral(L(lambda)*V(lambda) dlambda).
func PhotopicLuminance(g SpectralGrid, spec []unit.SpectralRadiance, vlambda *Passband) (unit.LuminanceCdM2, error) {
	return weightedLuminance(g, spec, vlambda, cieLuminousEfficacyLmPerW)
}

// ScotopicLuminance is PhotopicLuminance's scotopic analogue, weighted by
// the CIE scotopic V'(lambda) response.
func ScotopicLuminance(g SpectralGrid, spec []unit.SpectralRadiance, vPrimeLambda *Passband) (unit.LuminanceCdM2, error) {
	return weightedLuminance(g, spec, vPrimeLambda, cieScotopicPeakEfficacyLmPerW)
}

func weightedLuminance(g SpectralGrid, spec []unit.SpectralRadiance, resp *Passband, efficacy float64) (unit.LuminanceCdM2, error) {
	if len(spec) != g.Len() {
		return 0, fmt.Errorf("%w: spec length %d != grid length %d", ErrInvalidPassband, len(spec), g.Len())
	}

	r, err := resampleResponse(g, resp)
	if err != nil {
		return 0, err
	}

	w := g.Weights()

	integral := 0.0
	for i, l := range spec {
		integral += float64(l) * r[i] * w[i]
	}

	return unit.LuminanceCdM2(efficacy * integral), nil
}

// HorizontalIrradiance returns the bolometric (full-grid) irradiance on a
// horizontal surface: the sum over directions of each direction's
// wavelength-integrated radiance, projected by cos(zenith) = sin(altitude)
// and weighted by the direction's own solid angle. Directions at or below
// the horizon contribute nothing.
func HorizontalIrradiance(g SpectralGrid, f SpectralField, altitudesRad []float64, solidAngleSR []float64) (unit.Irradiance, error) {
	nDir, nLambda := f.Dims()
	if nLambda != g.Len() {
		return 0, fmt.Errorf("%w: field wavelength count %d != grid length %d", ErrInvalidPassband, nLambda, g.Len())
	}

	if len(altitudesRad) != nDir || len(solidAngleSR) != nDir {
		return 0, fmt.Errorf("%w: need %d altitudes/solid angles, got %d/%d",
			ErrInvalidPassband, nDir, len(altitudesRad), len(solidAngleSR))
	}

	w := g.Weights()

	total := 0.0

	for d := range nDir {
		cosZ := math.Sin(altitudesRad[d])
		if cosZ <= 0 {
			continue
		}

		row := f.Row(d)

		bolo := 0.0
		for k, v := range row {
			bolo += float64(v) * w[k]
		}

		total += bolo * cosZ * solidAngleSR[d]
	}

	return unit.Irradiance(total), nil
}
