// Package dataset assembles the reference data a [skybrightness.Preset] needs.
//
// # What this is for
//
// A preset names a configuration of components; it does not carry the data
// those components read. Gathering that data means a star map, a dust map, an
// airglow spectrum, a passband and — for the module's own model — a solar
// spectrum, each from a different service with its own client, its own cache
// and its own consent. Assembled by hand that is a page of code before the
// first sky brightness is computed, and the page is the same page every time.
//
// [Inputs] is that page. One call, one error to check, and the result goes
// straight into [skybrightness.NewPreset].
//
//	in, err := dataset.Inputs(ctx, dataset.Spec{Preset: skybrightness.GAMBONSWeb})
//	model, err := skybrightness.NewPreset(skybrightness.GAMBONSWeb, in)
//
// It also means a caller reaches one package rather than five. The
// subpackages stay exported and are the right thing when a caller wants one
// dataset, a different grid per component, or a source this does not offer;
// nothing here is a wrapper that hides them.
//
// # What it will not decide for you
//
// Downloads are not consented to here. [Endpoints] reports what a preset will
// fetch and how much it moves, so the grant stays where every other bulk fetch
// in this module puts it — an explicit [remote.EnableDownloads] at the call
// site:
//
//	ids, size := dataset.Endpoints(skybrightness.GAMBONSWeb)
//	remote.EnableDownloads(size, ids...)
//
// A convenience that granted its own consent would be a convenience that
// downloaded 145 MB because somebody typed a preset name.
//
// Ground emitters are not sourced either. [skybrightness.Observatory] needs an
// inventory of artificial light around the site, and satellite radiance alone
// cannot determine a source spectrum or an upward emission function — a
// default here would be reporting somebody else's city. Build one with
// [github.com/TuSKan/astrogo/skybrightness/dataset/viirs.Region] and pass it
// in [Spec.Emitters].
package dataset

import (
	"context"
	"errors"
	"fmt"

	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/skybrightness/dataset/airglow"
	"github.com/TuSKan/astrogo/skybrightness/dataset/dust"
	"github.com/TuSKan/astrogo/skybrightness/dataset/passband"
	"github.com/TuSKan/astrogo/skybrightness/dataset/solar"
	"github.com/TuSKan/astrogo/skybrightness/dataset/starlight"
	"github.com/TuSKan/astrogo/unit"
)

// ErrSpec reports a specification this package cannot satisfy.
var ErrSpec = errors.New("dataset: specification")

// DefaultBandID is the passband resolved when a spec names none.
//
// Johnson V, from the Spanish Virtual Observatory's calibration of the Bessell
// curves. V because it is the band nearly every published sky-brightness
// figure is quoted in, so it is the one a caller can compare against without
// converting anything.
const DefaultBandID = "Generic/Bessell.V"

// DefaultMapBand is the star-map column read when a spec names none, and is
// the same band [DefaultBandID] describes.
const DefaultMapBand = "V"

// DefaultStarTemperatureK is the blackbody temperature integrated starlight is
// given when a spec names none.
//
// # Why this is a choice and not a constant of nature
//
// Integrated starlight is the summed light of stars of every spectral type,
// and no single blackbody is right for it. This is a solar-like stand-in that
// puts the ensemble's colour in the right region, and it is a stated
// approximation rather than a measurement: a caller working in the blue, or
// comparing against a specific published spectrum, should set
// [Spec.StarTemperatureK] or build the shape themselves.
//
// It is exposed rather than buried for that reason. A default that cannot be
// seen is a default nobody knows to question.
const DefaultStarTemperatureK = 5800

// Spec says which preset the data is for, and how its inputs are sourced.
//
// # Spec against Option
//
// A [Sky] is configured from two places, which is one more than it looks like
// it should be. The division is that a Spec decides what data is fetched and
// from where, while an [Option] decides what the assembled Sky reports back.
//
// The line between them is not taste. A Spec field is allowed a documented
// zero-value default, because a caller who leaves one alone gets a stated
// choice they can look up. An Option exists precisely where that fails —
// where the zero value is itself a legitimate setting, so an unset field
// could not be told from a deliberate one. [WithMagSystem] is the case that
// forced it, and so far the only one.
//
// # Why there is no observer in it
//
// Almost nothing gathered here depends on where anybody stands: the star map
// and the dust map are all-sky, and the grid and passband are spectral. Those
// inputs serve every site at once, which is what lets one [Sky] answer for a
// whole network of them, and the observer that does matter is the one
// [Sky.Scene] takes.
//
// The airglow spectrum is the exception, and it is why this is [Observatory]
// rather than a location. SkyCalc models Paranal at three altitudes rather
// than an arbitrary site, so the place-dependence here cannot be resolved
// from coordinates — it can only be chosen, and a caller far from those
// altitudes needs to know they are choosing.
//
// A site field would have obscured exactly that. It would look like the
// answer to the one question this cannot answer, while every computation
// ignored it in favour of the observer arriving somewhere else entirely.
type Spec struct {
	// Preset is the configuration the inputs are gathered for. Required: it
	// decides whether a solar spectrum is fetched at all.
	Preset skybrightness.Preset

	// Grid is the spectral axis every component is sampled onto. Zero means
	// [skybrightness.DefaultOpticalGrid].
	Grid unit.SpectralGrid

	// BandID is the Spanish Virtual Observatory identifier of the passband
	// the star map is averaged over. Zero means [DefaultBandID].
	BandID string

	// MapBand is the column of the published star map to read, one of B, V,
	// R or I. Zero means V.
	//
	// Separate from BandID because the two are named by different
	// authorities and only coincidentally describe the same thing: SVO calls
	// Johnson V "Generic/Bessell.V" and the map calls it "V". Deriving one
	// from the other would work for the Bessell curves and quietly produce
	// the wrong column for anything else, so the pairing is stated. It is
	// checked against the map, and a mismatch names the bands on offer.
	MapBand string

	// Observatory selects the site SkyCalc models for airglow. It accepts
	// only the three SkyCalc offers — "paranal", "paranal-2400",
	// "paranal-3060" — because it is a Paranal model at three altitudes
	// rather than a general site calculator. Zero means paranal.
	//
	// A string rather than the airglow package's own type so that reaching
	// this package does not mean importing that one.
	Observatory string

	// StarTemperatureK is the blackbody temperature integrated starlight is
	// shaped by. Zero means [DefaultStarTemperatureK]; see the note there on
	// why this is an approximation.
	StarTemperatureK float64

	// StarShape overrides StarTemperatureK entirely, for a caller who has a
	// real ensemble spectrum rather than a blackbody. It must be on Grid.
	StarShape skybrightness.SpectralRadiance

	// SolarFluxSFU is the 10.7 cm solar radio flux that sets airglow's
	// overall level, in solar flux units. Zero means SkyCalc's own default
	// of 130. A caller modelling a specific night should use that night's
	// value, from the Canadian Space Weather Forecast Centre.
	SolarFluxSFU float64

	// AirglowScale multiplies the fetched airglow spectrum. Zero means 1.
	//
	// A flat scaling for a night known to be brighter or quieter than the
	// reference — see [airglow.Spec.Scale], and note that scaling a
	// climatology to match a measurement does not make it one.
	AirglowScale float64

	// AirglowMeasured records that the airglow spectrum describes the night
	// being modelled rather than a reference, which changes the quality flag
	// the component reports. False for anything this package fetches, since
	// SkyCalc is a model.
	AirglowMeasured bool

	// Emitters is the artificial-light inventory around the observer,
	// required by
	// [skybrightness.Observatory] and unused by the others. There is no
	// default and there cannot be one; see this package's own doc.
	Emitters []skybrightness.GroundEmitter
}

// Endpoints reports the remote endpoints a preset's data comes from, and the
// largest single fetch among them.
//
// The size is the per-download cap [remote.EnableDownloads] wants, not the
// total moved: consent is checked against one object at a time. The largest
// here is one hemisphere of the SFD dust map.
//
// Only endpoints that actually move a file are listed. The passband and
// airglow services answer queries rather than serving objects, so they need no
// consent and appear nowhere in the grant.
func Endpoints(p skybrightness.Preset) (ids []remote.EndpointID, maxSize int64) {
	ids = []remote.EndpointID{remote.GaiaStarMap, remote.SFDDustMap}

	// Only the module's own model has a moonlight term, and only that term
	// reads a solar spectrum.
	if p == skybrightness.Observatory {
		ids = append(ids, remote.CALSPEC)
	}

	for _, id := range ids {
		if e, ok := remote.Lookup(id); ok && e.ApproxSize > maxSize {
			maxSize = e.ApproxSize
		}
	}

	return ids, maxSize
}

// Inputs gathers everything a preset needs.
//
// Five services on a cold cache and none on a warm one, so an application
// pays this once at start-up and never again: the objects are immutable and
// are reused on existence alone.
//
// Returns [remote.ErrDownloadDenied] when consent has not been granted — see
// [Endpoints] for what to grant.
func Inputs(ctx context.Context, spec Spec) (skybrightness.PresetInputs, error) {
	var zero skybrightness.PresetInputs

	// Asking the preset for something only a real preset can answer, so an
	// unknown name fails here rather than after five downloads.
	if _, err := spec.Preset.DiffuseKappa(); err != nil {
		return zero, fmt.Errorf("dataset: %w", err)
	}

	grid := spec.Grid
	if grid.Validate() != nil {
		grid = skybrightness.DefaultOpticalGrid()
	}

	band, err := passband.Fetch(ctx, cmpOr(spec.BandID, DefaultBandID))
	if err != nil {
		return zero, fmt.Errorf("dataset: passband: %w", err)
	}

	starMap, err := starlight.Open(ctx)
	if err != nil {
		return zero, fmt.Errorf("dataset: star map: %w", err)
	}

	mapBand := cmpOr(spec.MapBand, DefaultMapBand)

	stars, err := starMap.Band(mapBand)
	if err != nil {
		return zero, fmt.Errorf("%w: the star map has no %q band; it carries %v",
			ErrSpec, mapBand, starMap.Bands())
	}

	shape := spec.StarShape
	if len(shape) == 0 {
		shape, err = skybrightness.BlackbodyShape(grid,
			cmpOr(spec.StarTemperatureK, DefaultStarTemperatureK))
		if err != nil {
			return zero, fmt.Errorf("dataset: starlight shape: %w", err)
		}
	}

	dustMap, err := dust.Open(ctx)
	if err != nil {
		return zero, fmt.Errorf("dataset: dust map: %w", err)
	}

	// The spectrum is fetched over the grid's own range, with a nanometre of
	// slack at each end so the resampling interpolates rather than
	// extrapolating at the edges.
	glow, err := airglow.Fetch(ctx, airglow.Spec{
		Observatory:  airglow.Observatory(spec.Observatory),
		SolarFluxSFU: spec.SolarFluxSFU,
		MinNM:        float64(grid.At(0)) - 1,
		MaxNM:        float64(grid.At(grid.Len()-1)) + 1,
		Scale:        spec.AirglowScale,
	})
	if err != nil {
		return zero, fmt.Errorf("dataset: airglow: %w", err)
	}

	in := skybrightness.PresetInputs{
		Stars:           stars,
		StarShape:       shape,
		Dust:            dustMap,
		AirglowZenith:   glow.Resample(grid),
		AirglowMeasured: spec.AirglowMeasured,
		Grid:            grid,
		Band:            band,
		Emitters:        spec.Emitters,
	}

	if spec.Preset != skybrightness.Observatory {
		return in, nil
	}

	if len(spec.Emitters) == 0 {
		return zero, fmt.Errorf("%w: %q needs a ground-emitter inventory; build one with "+
			"viirs.Region and pass it in Spec.Emitters", ErrSpec, spec.Preset)
	}

	sun, err := solar.Open(ctx)
	if err != nil {
		return zero, fmt.Errorf("dataset: solar spectrum: %w", err)
	}

	bands := magnitude.ROLOBands()

	in.SolarSpectrum = make([]float64, len(bands))
	if err := sun.Resample(in.SolarSpectrum, bands); err != nil {
		return zero, fmt.Errorf("dataset: solar spectrum: %w", err)
	}

	return in, nil
}

// cmpOr returns the first non-zero value, which Go 1.22 lacks and this file
// wants three times.
func cmpOr[T comparable](v, fallback T) T {
	var zero T
	if v == zero {
		return fallback
	}

	return v
}
