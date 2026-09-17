package dataset

import (
	"context"
	"errors"
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// Sky is a preset, its reference data and its transfer, assembled and ready
// to evaluate.
//
// # What it removes
//
// Getting a sky brightness by hand means granting consent, gathering five
// datasets, building a model, building an atmosphere that carries the
// preset's own kappa and scattering order, asking at the preset's own
// fidelity, and keeping the grid and passband to hand for every later call.
// Three of those steps are mechanical: they have exactly one correct answer,
// the caller cannot improve on them, and getting them wrong is rejected
// rather than corrected. A Sky holds them.
//
// What it does not hold is anything only the caller can know — where they
// are, when it is, and what the air was doing.
//
// # Why it makes scenes rather than holding one
//
// A [skybrightness.Scene] pins one instant. A service answering for many
// times would have to rebuild one per request and would be back to
// transcribing the transfer by hand, so [Sky.Scene] is a factory: it takes
// the atmosphere the caller describes and finishes it with the preset's
// transfer. Everything a Sky itself holds — model, grid, passband, fidelity —
// is independent of both time and place, so one Sky serves any number of
// nights and any number of sites.
//
// The single input that does carry a place is the airglow spectrum, which is
// a named SkyCalc model chosen through [Spec.Observatory] rather than derived
// from coordinates. A Sky evaluated far from that model's altitude is using
// it outside its range, and nothing in the geometry will say so.
type Sky struct {
	preset   skybrightness.Preset
	model    *skybrightness.Model
	inputs   skybrightness.PresetInputs
	fidelity skybrightness.Fidelity
	system   magnitude.System
}

// Option configures what an assembled [Sky] reports.
//
// The counterpart to [Spec], which configures what it fetches. See that type
// for why the two are separate rather than one struct with more fields.
type Option func(*Sky)

// WithMagSystem sets the magnitude system [Sky.SurfaceBrightness] and
// [Sky.Composition] report in. The default is [magnitude.Vega].
//
// An option rather than a [Spec] field because [magnitude.AB] is the zero
// value of [magnitude.System]: an unset field would silently mean AB, and sky
// brightness is quoted in Vega mag/arcsec^2 nearly everywhere it is quoted at
// all. A caller who left the field alone would get plausible numbers in the
// wrong system with nothing to say so.
func WithMagSystem(sys magnitude.System) Option {
	return func(s *Sky) { s.system = sys }
}

// Open assembles everything a preset needs and returns it ready to evaluate.
//
// Five services on a cold cache and none on a warm one, so an application
// pays this once at start-up.
//
// Consent is not granted here — see this package's own documentation for why,
// and [Endpoints] for what to grant. When it is missing this says so and
// names the call.
func Open(ctx context.Context, spec Spec, opts ...Option) (*Sky, error) {
	in, err := Inputs(ctx, spec)
	if err != nil {
		return nil, consentAdvice(spec.Preset, err)
	}

	return newSky(spec.Preset, in, opts...)
}

// newSky is everything Open does after the fetch.
//
// Split out so that the assembly — which model, which fidelity, which
// magnitude system, and which observer eventually reaches a scene — can be
// exercised without five services. Open is the only way a caller gets one,
// and that is deliberate; a test of the wiring around the inputs should not
// have to download 145 MB to reach it.
func newSky(
	p skybrightness.Preset, in skybrightness.PresetInputs, opts ...Option,
) (*Sky, error) {
	model, err := skybrightness.NewPreset(p, in)
	if err != nil {
		return nil, fmt.Errorf("dataset: %w", err)
	}

	fidelity, err := p.Fidelity()
	if err != nil {
		return nil, fmt.Errorf("dataset: %w", err)
	}

	s := &Sky{
		preset:   p,
		model:    model,
		inputs:   in,
		fidelity: fidelity,
		system:   magnitude.Vega,
	}

	for _, o := range opts {
		o(s)
	}

	return s, nil
}

// consentAdvice turns a refused download into the two lines that grant it.
//
// The friction was never typing them; it was not knowing they were needed.
// A caller who has just been told what to paste is in a better position than
// one handed a policy error, and this keeps the grant at their call site
// where the design requires it to be.
func consentAdvice(p skybrightness.Preset, err error) error {
	if !errors.Is(err, remote.ErrDownloadDenied) {
		return err
	}

	_, size := Endpoints(p)

	return fmt.Errorf("%w\n\npreset %q needs up to %d MB of reference data and downloads "+
		"are not enabled. Grant it with:\n\n"+
		"\tids, size := dataset.Endpoints(%q)\n"+
		"\tremote.EnableDownloads(size, ids...)",
		err, p, size>>20, p)
}

// Scene builds the physical state to evaluate under, with the preset's
// transfer already applied.
//
// The caller brings what only they know: where, when, and the air. `air` is
// an [atmosphere.Builder] they have filled in — an aerosol regime, cloud, a
// horizon — and this finishes it with the preset's kappa and scattering
// order, which they must not choose. Pass nil to accept a clear atmosphere at
// the site's own elevation.
//
// # Surface conditions are yours, and easy
//
// A nil builder means a clear atmosphere at the site's own elevation, with
// surface pressure and temperature from the ICAO standard profile. A non-nil
// one is used exactly as given, and should carry
// [atmosphere.Builder.SurfaceAtAltitude] with the site's height unless the
// caller has real readings.
//
// This does not fill them in behind a caller's back, and the reason is worth
// stating: [atmosphere.NewBuilder] starts every builder at *sea-level*
// standard conditions, so there is no way to tell a caller who wants sea
// level from one who never thought about it. Guessing between them would mean
// silently overriding an explicit choice, or silently keeping a wrong one. A
// named call the caller makes is better than either, and
// SurfaceAtAltitude(site.Height()) is not the kind of line anyone gets wrong.
func (s *Sky) Scene(
	site *coord.Geodetic, when time.GoTime, air *atmosphere.Builder,
) (*skybrightness.Scene, error) {
	if site == nil {
		return nil, fmt.Errorf("%w: a scene needs a site", ErrSpec)
	}

	if air == nil {
		// Nil is unambiguous: the caller has expressed no atmosphere at all,
		// so a clear one at the right elevation is a default rather than an
		// override.
		air = atmosphere.NewBuilder().SurfaceAtAltitude(site.Height())
	}

	air, err := s.preset.Transfer(air)
	if err != nil {
		return nil, fmt.Errorf("dataset: %w", err)
	}

	built, err := air.Build()
	if err != nil {
		return nil, fmt.Errorf("dataset: atmosphere: %w", err)
	}

	return &skybrightness.Scene{
		Observer:   site,
		Time:       when,
		Atmosphere: built,
		Ephemeris:  eph.Default(),
	}, nil
}

// Direction evaluates the sky in one direction, at the preset's own fidelity
// and on its own grid.
func (s *Sky) Direction(
	ctx context.Context, scene *skybrightness.Scene, alt, az angle.Angle,
) (*skybrightness.Estimate, error) {
	est, err := s.model.Direction(ctx, s.query(scene), alt, az)
	if err != nil {
		return nil, fmt.Errorf("dataset: %w", err)
	}

	return est, nil
}

// Zenith evaluates straight up.
func (s *Sky) Zenith(
	ctx context.Context, scene *skybrightness.Scene,
) (*skybrightness.Estimate, error) {
	est, err := s.model.Zenith(ctx, s.query(scene))
	if err != nil {
		return nil, fmt.Errorf("dataset: %w", err)
	}

	return est, nil
}

// SkyMap evaluates the whole hemisphere, in rings of equal altitude.
//
// The result feeds [skybrightness.HorizontalIlluminance] and
// [skybrightness.IntegratedHemisphere] directly.
func (s *Sky) SkyMap(
	ctx context.Context, scene *skybrightness.Scene, rings int,
) ([]skybrightness.SkyPoint, error) {
	points, err := s.model.SkyMap(ctx, s.query(scene), rings)
	if err != nil {
		return nil, fmt.Errorf("dataset: %w", err)
	}

	return points, nil
}

// SurfaceBrightness projects an estimate through this Sky's own passband and
// magnitude system.
func (s *Sky) SurfaceBrightness(est *skybrightness.Estimate) (float64, error) {
	v, err := est.SurfaceBrightness(s.inputs.Band, s.system)
	if err != nil {
		return 0, fmt.Errorf("dataset: %w", err)
	}

	return v, nil
}

// Composition reports what a sky is made of, through this Sky's own passband
// and magnitude system. Brightest term first.
func (s *Sky) Composition(est *skybrightness.Estimate) ([]skybrightness.ComponentShare, error) {
	rows, err := est.Composition(s.inputs.Band, s.system)
	if err != nil {
		return nil, fmt.Errorf("dataset: %w", err)
	}

	return rows, nil
}

// Model returns the assembled model, for a caller who needs something this
// type does not expose. Nothing here is a wall.
func (s *Sky) Model() *skybrightness.Model { return s.model }

// Preset reports which configuration this Sky was built from.
func (s *Sky) Preset() skybrightness.Preset { return s.preset }

// Grid is the spectral axis every evaluation runs on.
func (s *Sky) Grid() unit.SpectralGrid { return s.inputs.Grid }

// Band is the passband projections are taken through.
func (s *Sky) Band() magnitude.Passband { return s.inputs.Band }

// Inputs returns the assembled reference data, for a caller who wants to
// build a second model from it rather than re-fetch.
func (s *Sky) Inputs() skybrightness.PresetInputs { return s.inputs }

// query is the evaluation request this Sky makes, with the fidelity and grid
// the preset defines rather than whatever a caller remembered.
func (s *Sky) query(scene *skybrightness.Scene) skybrightness.Query {
	return skybrightness.Query{
		Scene:    scene,
		Grid:     s.inputs.Grid,
		Fidelity: s.fidelity,
	}
}
