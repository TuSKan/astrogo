// Package airglow supplies the zenith airglow spectrum that
// [github.com/TuSKan/astrogo/skybrightness.Airglow] needs and refuses to guess.
//
// Airglow is the one natural component with no useful default. It is chemical
// emission from the upper atmosphere, it varies with solar activity, season,
// time of night and site, and its spectrum is a forest of OH and O2 bands
// rather than anything a blackbody or a constant approximates. That is why the
// component takes a caller-supplied zenith spectrum: the alternative is
// inventing one, which this module does not do.
//
// The spectrum comes from ESO's Cerro Paranal Advanced Sky Model — SkyCalc —
// which is the same source GAMBONS uses for its own airglow term. Evaluation
// performs no I/O, so fetching happens here and the result is handed to the
// component.
package airglow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"strings"

	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/fits"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/remote/api"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/unit"
)

// Sentinel errors for the airglow provider.
var (
	// ErrSpec is returned for a request the service would reject.
	ErrSpec = errors.New("airglow: unusable request")

	// ErrService is returned when SkyCalc answers with something this
	// package cannot use, including its own validation errors.
	ErrService = errors.New("airglow: unusable response from SkyCalc")
)

// Observatory names a site SkyCalc models.
//
// These are the only three it accepts; it is a Paranal model with two
// alternative altitudes rather than a general site calculator.
type Observatory string

// The sites SkyCalc supports.
const (
	Paranal      Observatory = "paranal" // 2640 m
	LaSilla      Observatory = "lasilla" // 2400 m
	Altitude3060 Observatory = "3060m"   // 3060 m
)

// Spec is a request for a zenith airglow spectrum.
type Spec struct {
	// Observatory is the site. Defaults to [Paranal].
	Observatory Observatory

	// SolarFluxSFU is the monthly averaged solar radio flux at 10.7 cm, in
	// solar flux units, which is what sets airglow's overall level.
	//
	// SkyCalc's own default is 130. GAMBONS ships its reference spectrum at
	// 100. Current values come from the Canadian Space Weather Forecast
	// Centre; a caller modelling a specific night should use that night's.
	// Zero means SkyCalc's default.
	SolarFluxSFU float64

	// MinNM and MaxNM bound the spectrum. Zero means the grid this is being
	// fetched for, or SkyCalc's 300-2000 nm if there is no grid.
	MinNM, MaxNM float64

	// StepNM is the wavelength sampling. Zero means 0.1 nm, SkyCalc's own
	// default, which resolves the OH bands.
	StepNM float64
}

// skycalcRequest is the JSON body SkyCalc's API takes.
//
// # Every field, not the interesting ones
//
// The service rejects a partial body with a 500 rather than filling the gaps
// from its own defaults, which is worth knowing because the natural
// implementation — send what you want to change — fails against it. Measured:
// a fifteen-field request returns "Internal Server Error"; the same request
// padded to all thirty-five returns a job.
//
// So the defaults are transcribed here from ESO's own published
// skycalc_defaults.json, and [Spec] overrides a handful of them. Everything
// except airglow is switched off, so what comes back is the airglow term alone
// rather than a whole sky the caller would have to subtract from.
type skycalcRequest struct {
	Airmass float64 `json:"airmass"`

	PWVMode string  `json:"pwv_mode"`
	Season  int     `json:"season"`
	Time    int     `json:"time"`
	PWV     float64 `json:"pwv"`

	SolarFlux float64 `json:"msolflux"`

	InclMoon      string  `json:"incl_moon"`
	MoonSunSep    float64 `json:"moon_sun_sep"`
	MoonTargetSep float64 `json:"moon_target_sep"`
	MoonAlt       float64 `json:"moon_alt"`
	MoonEarthDist float64 `json:"moon_earth_dist"`

	InclStarlight string  `json:"incl_starlight"`
	InclZodiacal  string  `json:"incl_zodiacal"`
	EclLon        float64 `json:"ecl_lon"`
	EclLat        float64 `json:"ecl_lat"`

	InclLowerAtm string `json:"incl_loweratm"`
	InclUpperAtm string `json:"incl_upperatm"`
	InclAirglow  string `json:"incl_airglow"`

	InclThermal string  `json:"incl_therm"`
	ThermT1     float64 `json:"therm_t1"`
	ThermE1     float64 `json:"therm_e1"`
	ThermT2     float64 `json:"therm_t2"`
	ThermE2     float64 `json:"therm_e2"`
	ThermT3     float64 `json:"therm_t3"`
	ThermE3     float64 `json:"therm_e3"`

	VacAir    string  `json:"vacair"`
	WMin      float64 `json:"wmin"`
	WMax      float64 `json:"wmax"`
	WGridMode string  `json:"wgrid_mode"`
	WDelta    float64 `json:"wdelta"`
	WRes      int     `json:"wres"`

	LSFType       string  `json:"lsf_type"`
	LSFGaussFWHM  float64 `json:"lsf_gauss_fwhm"`
	LSFBoxcarFWHM float64 `json:"lsf_boxcar_fwhm"`

	Observatory string `json:"observatory"`
}

// skycalcDefaults returns ESO's published defaults, from
// https://www.eso.org/observing/etc/doc/skycalc/skycalc_defaults.json.
//
// The moon, zodiacal and thermal fields are carried even though those
// components are switched off, because the service wants the keys present
// regardless of whether it will use them.
func skycalcDefaults() skycalcRequest {
	return skycalcRequest{
		Airmass:       1.0,
		PWVMode:       "pwv",
		Season:        0,
		Time:          0,
		PWV:           3.5,
		SolarFlux:     130.0,
		InclMoon:      "Y",
		MoonSunSep:    90.0,
		MoonTargetSep: 45.0,
		MoonAlt:       45.0,
		MoonEarthDist: 1.0,
		InclStarlight: "Y",
		InclZodiacal:  "Y",
		EclLon:        135.0,
		EclLat:        90.0,
		InclLowerAtm:  "Y",
		InclUpperAtm:  "Y",
		InclAirglow:   "Y",
		InclThermal:   "N",
		VacAir:        "vac",
		WMin:          300.0,
		WMax:          2000.0,
		WGridMode:     "fixed_wavelength_step",
		WDelta:        0.1,
		WRes:          20000,
		LSFType:       "none",
		LSFGaussFWHM:  5.0,
		LSFBoxcarFWHM: 5.0,
		Observatory:   "paranal",
	}
}

// skycalcResponse is what the POST returns.
type skycalcResponse struct {
	Status string `json:"status"`
	TmpDir string `json:"tmpdir"`
	Error  string `json:"error"`
}

// Spectrum is a zenith airglow spectrum, in spectral radiance.
type Spectrum struct {
	// LambdaNM ascends.
	LambdaNM []float64

	// Radiance is W m^-2 sr^-1 nm^-1 at each wavelength, converted from the
	// photon-flux-per-square-arcsecond SkyCalc reports.
	Radiance []float64

	// Source records the request, for provenance.
	Source string
}

// Fetch retrieves a zenith airglow spectrum from ESO SkyCalc.
//
// # The three calls
//
// SkyCalc's API runs the model on a POST, writes the result into a temporary
// directory on its own server, and hands back the directory name. The FITS is
// then a plain GET, and a third call releases the directory. All three happen
// here, and the third happens even when the second fails, because ESO
// otherwise accumulates a directory per request from every caller who did not
// bother.
//
// Airmass is fixed at 1: the component this feeds applies van Rhijn itself, so
// what it needs is the zenith spectrum and asking SkyCalc to tilt it too would
// count the path length twice.
func Fetch(ctx context.Context, spec Spec) (*Spectrum, error) {
	req, err := spec.request()
	if err != nil {
		return nil, err
	}

	client, err := api.NewClient(remote.ESOSkyCalc)
	if err != nil {
		return nil, fmt.Errorf("airglow: client: %w", err)
	}

	defer func() { _ = client.Close() }()

	posted, err := client.PostJSON(ctx, remote.ESOSkyCalc, "api/skycalc", req)
	if err != nil {
		return nil, fmt.Errorf("airglow: skycalc request: %w", err)
	}

	var res skycalcResponse

	decodeErr := json.NewDecoder(posted).Decode(&res)
	_ = posted.Close()

	if decodeErr != nil {
		return nil, fmt.Errorf("%w: decoding the job response: %w", ErrService, decodeErr)
	}

	if res.Status != "success" {
		return nil, fmt.Errorf("%w: status %q: %s", ErrService, res.Status, res.Error)
	}

	if res.TmpDir == "" {
		return nil, fmt.Errorf("%w: success without a temporary directory", ErrService)
	}

	// Release the server's temporary directory whatever happens next.
	defer func() { releaseTmpDir(ctx, client, res.TmpDir) }()

	body, err := client.Get(ctx, remote.ESOSkyCalc, "tmp/"+res.TmpDir+"/skytable.fits", nil)
	if err != nil {
		return nil, fmt.Errorf("airglow: retrieving skytable.fits: %w", err)
	}

	defer func() { _ = body.Close() }()

	spectrum, err := Parse(body)
	if err != nil {
		return nil, err
	}

	spectrum.Source = spec.describe()

	return spectrum, nil
}

// Parse reads a SkyCalc skytable.fits into a spectrum.
//
// # Which columns, and why not FLUX
//
// The table carries a FLUX column that is the whole modelled sky. This reads
// FLUX_AEL and FLUX_ARC instead — the airglow emission lines and the airglow
// residual continuum — and adds them. Using FLUX would fold in whatever else
// the service included and hand the caller a sky to subtract from rather than
// a component to add.
//
// SkyCalc reports photons s^-1 m^-2 um^-1 arcsec^-2. Spectral radiance is that
// divided by a thousand for micrometres to nanometres, divided by the solid
// angle of a square arcsecond, and multiplied by the energy of one photon at
// its own wavelength. Skipping the last step leaves a photon count that looks
// like a radiance.
func Parse(r io.Reader) (*Spectrum, error) {
	f, err := fits.Read(r)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrService, err)
	}

	lambda, lines, continuum, err := columns(f)
	if err != nil {
		return nil, err
	}

	// All three columns are read from one table and so carry NumRows entries
	// each, but the shortest is taken rather than assumed: a response is not a
	// file this package wrote.
	rows := min(len(lambda), min(len(lines), len(continuum)))

	out := &Spectrum{
		LambdaNM: make([]float64, 0, rows),
		Radiance: make([]float64, 0, rows),
	}

	for i := range rows {
		nm := lambda[i]

		// A sample that is not a number is dropped rather than carried. A NaN
		// here does not stay local: it becomes a NaN radiance, which sums into
		// the scene's total and comes back out as a NaN magnitude, by which
		// point nothing identifies where it entered. This is the same
		// screening solar.Parse does on the CALSPEC spectrum.
		if nm <= 0 || math.IsNaN(nm) || math.IsInf(nm, 0) {
			continue
		}

		flux := lines[i] + continuum[i]
		if math.IsNaN(flux) || math.IsInf(flux, 0) {
			continue
		}

		// A negative airglow is a subtraction artefact in the model that
		// produced it, not emission removed from the sky.
		if flux < 0 {
			flux = 0
		}

		perNMPerSr := flux / 1000 / constants.ArcsecondSquaredToSteradian

		out.LambdaNM = append(out.LambdaNM, nm)
		out.Radiance = append(out.Radiance, float64(constants.ToEnergy(
			unit.PhotonSpectralRadiance(perNMPerSr), unit.WavelengthNM(nm))))
	}

	// At interpolates between samples and needs two to do it; one row is not a
	// spectrum, and zero rows means the table held no usable data at all.
	if len(out.LambdaNM) < 2 {
		return nil, fmt.Errorf("%w: %d usable rows", ErrService, len(out.LambdaNM))
	}

	return out, nil
}

// At interpolates the spectrum, returning zero outside its range.
//
// Zero rather than the endpoint, unlike the extragalactic background: an
// airglow spectrum is a line forest, so holding the last value flat would
// continue whichever band happened to be at the edge across everything beyond
// it, which is worse than admitting there is no data.
func (s *Spectrum) At(lambdaNM float64) float64 {
	n := len(s.LambdaNM)
	if n == 0 || lambdaNM < s.LambdaNM[0] || lambdaNM > s.LambdaNM[n-1] {
		return 0
	}

	lo, hi := 0, n-1
	for hi-lo > 1 {
		mid := (lo + hi) / 2
		if s.LambdaNM[mid] <= lambdaNM {
			lo = mid
		} else {
			hi = mid
		}
	}

	span := s.LambdaNM[hi] - s.LambdaNM[lo]
	if span <= 0 {
		return s.Radiance[lo]
	}

	t := (lambdaNM - s.LambdaNM[lo]) / span

	return s.Radiance[lo] + t*(s.Radiance[hi]-s.Radiance[lo])
}

// Resample projects the spectrum onto a grid.
func (s *Spectrum) Resample(grid unit.SpectralGrid) skybrightness.SpectralRadiance {
	dst := skybrightness.NewSpectralRadiance(grid)
	for i := range dst {
		dst[i] = s.At(float64(grid.At(i)))
	}

	return dst
}

// NewAirglow fetches a zenith spectrum and builds the component over it.
//
// layerHeightM is the emitting layer's height for the van Rhijn geometry the
// component applies; the OH layer sits near 87 km.
//
// The result is flagged [github.com/TuSKan/astrogo/skybrightness.SolarAdjustedAirglow]
// rather than measured, because a SkyCalc spectrum is a model scaled by a solar
// index and not an observation of the night in question.
func NewAirglow(
	ctx context.Context,
	spec Spec,
	grid unit.SpectralGrid,
	layerHeightM float64,
) (*skybrightness.Airglow, error) {
	if spec.MinNM == 0 && spec.MaxNM == 0 && grid.Len() > 0 {
		// Ask for exactly the grid being evaluated, plus a nanometre either
		// side so the endpoints interpolate rather than fall off.
		spec.MinNM = float64(grid.At(0)) - 1
		spec.MaxNM = float64(grid.At(grid.Len()-1)) + 1
	}

	spectrum, err := Fetch(ctx, spec)
	if err != nil {
		return nil, err
	}

	component, err := skybrightness.NewAirglow(spectrum.Resample(grid), grid, layerHeightM, false)
	if err != nil {
		return nil, fmt.Errorf("airglow: building the component: %w", err)
	}

	return component, nil
}

// request builds and validates the JSON body.
func (s Spec) request() (skycalcRequest, error) {
	obs := s.Observatory
	if obs == "" {
		obs = Paranal
	}

	switch obs {
	case Paranal, LaSilla, Altitude3060:
	default:
		return skycalcRequest{}, fmt.Errorf("%w: observatory %q is not one SkyCalc models", ErrSpec, obs)
	}

	flux := s.SolarFluxSFU
	if flux == 0 {
		flux = 130 // SkyCalc's own default
	}

	if flux < 0 || math.IsNaN(flux) {
		return skycalcRequest{}, fmt.Errorf("%w: solar flux %v sfu", ErrSpec, flux)
	}

	minNM, maxNM := s.MinNM, s.MaxNM
	if minNM == 0 && maxNM == 0 {
		minNM, maxNM = 300, 2000
	}

	// SkyCalc's own stated bounds. Asking outside them is a rejected request
	// and a wasted round trip.
	if minNM < 300 || maxNM > 30000 || minNM >= maxNM {
		return skycalcRequest{}, fmt.Errorf("%w: %g-%g nm is outside SkyCalc's 300-30000 nm",
			ErrSpec, minNM, maxNM)
	}

	step := s.StepNM
	if step == 0 {
		step = 0.1
	}

	if step <= 0 {
		return skycalcRequest{}, fmt.Errorf("%w: wavelength step %v nm", ErrSpec, step)
	}

	req := skycalcDefaults()

	// Airmass 1 because the component applies van Rhijn itself.
	req.Airmass = 1.0
	req.Observatory = string(obs)
	req.SolarFlux = flux

	// Everything that is not airglow, off.
	req.InclMoon = "N"
	req.InclStarlight = "N"
	req.InclZodiacal = "N"
	req.InclThermal = "N"
	req.InclAirglow = "Y"

	req.WMin = minNM
	req.WMax = maxNM
	req.WGridMode = "fixed_wavelength_step"
	req.WDelta = step
	req.LSFType = "none"

	return req, nil
}

// describe renders the request for the spectrum's provenance line.
func (s Spec) describe() string {
	obs := s.Observatory
	if obs == "" {
		obs = Paranal
	}

	flux := s.SolarFluxSFU
	if flux == 0 {
		flux = 130
	}

	return fmt.Sprintf("ESO SkyCalc, %s, airmass 1, msolflux %g sfu", obs, flux)
}

// releaseTmpDir asks the service to delete the directory it wrote into.
//
// Failure is ignored deliberately. The caller wanted a spectrum, and losing one
// because the cleanup call was refused would be the wrong trade; the request
// still went out, which is the part that matters to ESO.
func releaseTmpDir(ctx context.Context, client *api.Client, tmpdir string) {
	body, err := client.Get(ctx, remote.ESOSkyCalc, "api/rmtmp", url.Values{"d": {tmpdir}})
	if err != nil {
		return
	}

	_, _ = io.Copy(io.Discard, body)
	_ = body.Close()
}

// floatColumn reads a column whatever case the service wrote its name in.
//
// ESO's documentation names these columns LAM, FLUX_AEL and FLUX_ARC. The
// service writes lam, flux_ael and flux_arc. Asking for the documented spelling
// finds nothing, and the failure looks like "no binary table in the response"
// rather than "wrong case", which is why this tries both rather than trusting
// either.
func floatColumn(table *fits.BintableHDU, name string) ([]float64, error) {
	v, err := table.GetFloatColumn(name)
	if err == nil {
		return v, nil
	}

	if upper, upErr := table.GetFloatColumn(strings.ToUpper(name)); upErr == nil {
		return upper, nil
	}

	return nil, fmt.Errorf("%w: column %q: %w", ErrService, name, err)
}

// columns pulls the wavelength and the two airglow terms out of the table.
func columns(f *fits.File) (lambda, lines, continuum []float64, err error) {
	for _, hdu := range f.HDUs {
		table, ok := hdu.(*fits.BintableHDU)
		if !ok {
			continue
		}

		lambda, err = floatColumn(table, "lam")
		if err != nil {
			continue
		}

		lines, err = floatColumn(table, "flux_ael")
		if err != nil {
			return nil, nil, nil, fmt.Errorf("%w: no airglow emission-line column: %w", ErrService, err)
		}

		continuum, err = floatColumn(table, "flux_arc")
		if err != nil {
			return nil, nil, nil, fmt.Errorf("%w: no airglow continuum column: %w", ErrService, err)
		}

		if len(lambda) != len(lines) || len(lambda) != len(continuum) {
			return nil, nil, nil, fmt.Errorf("%w: %d wavelengths against %d and %d fluxes",
				ErrService, len(lambda), len(lines), len(continuum))
		}

		if len(lambda) == 0 {
			return nil, nil, nil, fmt.Errorf("%w: the table is empty", ErrService)
		}

		// SkyCalc's LAM is in nanometres when the request was, but the header
		// is what says so; a table in micrometres would be off by a thousand
		// and still look plausible.
		if lambda[len(lambda)-1] < 30 {
			for i := range lambda {
				lambda[i] *= 1000
			}
		}

		return lambda, lines, continuum, nil
	}

	return nil, nil, nil, fmt.Errorf("%w: no binary table in the response", ErrService)
}
