package skybrightness

import (
	"errors"
	"fmt"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/unit"
)

// ErrPreset is returned when a preset cannot be built from what it was given.
var ErrPreset = errors.New("skybrightness: preset")

// Preset names a published configuration of components and radiative
// transfer, so that a caller can ask for one by name instead of assembling it
// and hoping they matched somebody's paper.
//
// A preset is a thin, readable constructor and nothing more. It registers a
// set of components and reports the transfer parameters that belong with them;
// it holds no hidden state, resolves no data, and every value it sets is
// traceable to a reference in its own documentation. The estimate it produces
// records which preset produced it, so a number can be traced back to the
// configuration that generated it rather than to a shape somebody once built
// in a test.
//
// The two below are deliberately the whole set. A library of named
// configurations becomes a place where defaults hide, which is the opposite of
// what this one is for; a caller wanting something else builds it from
// [NewModel] directly, and the preset's own source is the worked example of
// how.
type Preset string

const (
	// GAMBONSWeb reproduces the GAMBONS web service at gambons.fqa.ub.edu.
	//
	// Not GAMBONS itself, and the distinction matters. Masana et al. (2024)
	// Section 5 describe two models: Eq. 11, a scattering integral over the
	// whole hemisphere for every direction of observation, used for what they
	// call routine sky brightness calculations; and a simplification for the
	// web service, which replaces the optical depth with an effective one,
	// tau_eff = kappa * tau, to make the computation fast enough to answer a
	// web request. This preset is the second.
	//
	// They are not interchangeable. The paper states plainly that the
	// effective depth "cannot exactly reproduce the scattering model described
	// by Eq. 11 using a single kappa value", and that the simplified model
	// runs bright near the horizon and dark at the zenith, by under a tenth of
	// a magnitude for most cases. A comparison against a published GAMBONS
	// result therefore has to know which of the two produced it: Table 2 of
	// that paper is a full-model zenith composition, and this preset is not
	// expected to reproduce it exactly.
	//
	// What it sets: the five natural components of Eq. 10 and nothing else,
	// since GAMBONS models a moonless night with no artificial light; airglow
	// on van Rhijn geometry with the emitting layer at 87 km after Hart
	// (2019a); and kappa = 0.5, the web service's own value.
	GAMBONSWeb Preset = "gambons-web"

	// NaturalSky is this module's own configuration for the natural night sky.
	//
	// The same five components and the same equations, differing from
	// [GAMBONSWeb] only where a difference was measured to be an improvement
	// rather than assumed to be one:
	//
	//   - kappa is 0.75 after Duriscoe (2013), following Kwon (1989), rather
	//     than the web service's 0.5. Hong et al. (1998) put the range at 0.5
	//     to 0.9 and make it a function of the aerosol albedo and asymmetry
	//     parameter, which is the right way to set it and is not yet done
	//     here; 0.75 is the published midpoint until it is.
	//   - the airmass is Pickering (2002) rather than Kasten & Young (1989).
	//     Measured, the two agree to better than three parts in a thousand
	//     above five degrees of altitude — no hundredths of a magnitude in any
	//     band — and diverge only in the last degrees, where Pickering is the
	//     better behaved.
	//
	// It does not add moonlight or artificial skyglow. Those are components a
	// caller registers alongside these when the scene calls for them, not
	// properties of the natural sky, and a preset that quietly included them
	// would answer a different question from the one asked.
	NaturalSky Preset = "natural-sky"
)

// DiffuseKappa returns the effective-optical-depth factor this preset uses,
// ready for [github.com/TuSKan/astrogo/atmosphere.Builder.DiffuseScattering].
//
// It is reported rather than applied because the factor is a property of the
// atmosphere, not of the component set, and the scene owns the atmosphere. A
// caller building a scene for a preset sets it there; one who does not gets
// the atmosphere package's own default and should know which they have.
func (p Preset) DiffuseKappa() (float64, error) {
	switch p {
	case GAMBONSWeb:
		return 0.5, nil
	case NaturalSky:
		return 0.75, nil
	default:
		return 0, fmt.Errorf("%w: unknown preset %q", ErrPreset, p)
	}
}

// PresetInputs carries the data a preset cannot resolve for itself.
//
// Evaluation performs no I/O, so every dataset a component needs arrives
// here, fetched by the caller from the providers under dataset/. A preset
// chooses the components and the parameters; it does not choose the data,
// because which star map, which dust map and above all which airglow spectrum
// are the caller's to decide and the airglow one is a free parameter of the
// model rather than a prediction by it.
type PresetInputs struct {
	// Stars is the extra-atmospheric integrated starlight map, in the same
	// passband as Band.
	Stars StarMap

	// StarShape is the spectral shape starlight is given. Integrated
	// starlight is the summed light of stars of every type and no single
	// blackbody is right, so this is supplied rather than guessed.
	StarShape SpectralRadiance

	// Dust is the 100 micron map diffuse galactic light correlates against.
	Dust DustMap

	// AirglowZenith is the zenith airglow spectrum at the emitting layer.
	AirglowZenith SpectralRadiance

	// AirglowMeasured records that the spectrum came from an observation of
	// the night being modelled rather than from a reference, which changes
	// the quality flag the component reports.
	AirglowMeasured bool

	// Grid and Band are the spectral grid everything is evaluated on and the
	// passband the star map is averaged over.
	Grid unit.SpectralGrid
	Band magnitude.Passband
}

// NewPreset builds the model a preset specifies.
//
// The components are registered in the order Masana et al. (2021) Eq. 10 lists
// them — integrated starlight, diffuse galactic light, the extragalactic
// background, zodiacal light and airglow — which is cosmetic, since radiance
// sums, but keeps a breakdown printed from an estimate reading in the same
// order as the paper it is being checked against.
//
// A caller wanting moonlight or artificial skyglow registers those with
// [NewModel] alongside what this returns; see the note on [NaturalSky].
func NewPreset(p Preset, in PresetInputs) (*Model, error) {
	if _, err := p.DiffuseKappa(); err != nil {
		return nil, err
	}

	if err := in.Grid.Validate(); err != nil {
		return nil, fmt.Errorf("%w %q: %w", ErrPreset, p, err)
	}

	switch {
	case in.Stars == nil:
		return nil, fmt.Errorf("%w %q: needs a star map", ErrPreset, p)
	case in.Dust == nil:
		return nil, fmt.Errorf("%w %q: needs a dust map", ErrPreset, p)
	case len(in.AirglowZenith) == 0:
		return nil, fmt.Errorf("%w %q: needs a zenith airglow spectrum", ErrPreset, p)
	case len(in.StarShape) == 0:
		return nil, fmt.Errorf("%w %q: needs a starlight spectral shape", ErrPreset, p)
	}

	starlight, err := NewIntegratedStarlight(in.Stars, in.StarShape, in.Grid, in.Band)
	if err != nil {
		return nil, fmt.Errorf("%w %q: %w", ErrPreset, p, err)
	}

	galactic, err := NewDiffuseGalacticLight(in.Dust, in.Stars, in.Band)
	if err != nil {
		return nil, fmt.Errorf("%w %q: %w", ErrPreset, p, err)
	}

	// The emitting layer at 87 km, which both presets take from Hart (2019a)
	// by way of Masana et al. (2021) Section 6.
	glow, err := NewAirglow(in.AirglowZenith, in.Grid, atmosphere.AirglowLayerHeightM, in.AirglowMeasured)
	if err != nil {
		return nil, fmt.Errorf("%w %q: %w", ErrPreset, p, err)
	}

	model, err := NewModel(string(p),
		starlight, galactic, NewExtragalacticBackground(), NewZodiacalLight(), glow)
	if err != nil {
		return nil, fmt.Errorf("%w %q: %w", ErrPreset, p, err)
	}

	return model, nil
}
