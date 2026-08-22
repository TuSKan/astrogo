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

	// GAMBONSFull is GAMBONS as the paper computes it, not as the web service
	// does.
	//
	// Masana et al. (2024) Section 5 describe both. [GAMBONSWeb] is the
	// simplification; this is Eq. 11, the scattering integral over the whole
	// hemisphere for every direction of observation, which they use "for
	// routine sky brightness calculations" and which is available in their
	// stand-alone version. The two are different models and neither is a
	// tuning of the other.
	//
	// What it sets: the same five natural components, and kappa = 1. That
	// kappa is not a choice about scattering, it is the absence of one — with
	// no effective-depth stand-in the components apply the true extinction and
	// nothing else, which is exactly the direct term L_d of Eq. 8. The
	// scattered term L_s comes from the integral, which [Model.Estimate] adds
	// when the query asks for [Reference] fidelity. Use [Preset.Fidelity] to
	// get the level a preset expects rather than assuming it.
	//
	// A query at [Standard] fidelity against this preset is not a cheaper
	// version of it, it is a sky with no scattering treatment at all and will
	// come out too faint. That is the one way to hold this preset wrong, and
	// it is why the fidelity is reported rather than left to the caller's
	// memory.
	//
	// # What it is validated against
	//
	// Table 2 of the same paper, which is a full-model zenith composition.
	// That table was never a target [GAMBONSWeb] could be held to and is the
	// right one here. Conversely the published all-sky export is a web-version
	// run, so it validates [GAMBONSWeb] and cannot validate this.
	GAMBONSFull Preset = "gambons-full"

	// Observatory is this module's own model, and the only preset that is not
	// trying to be somebody else.
	//
	// The other three reproduce a published model, which means their job is to
	// match it including where it is approximate. This one's job is to be as
	// right as the module can make it, so it departs from GAMBONS wherever a
	// departure was measured to be an improvement rather than assumed to be
	// one:
	//
	//   - The full Eq. 11 scattering integral, with kappa = 1 so the direct
	//     term is true extinction. Measured against Table 2 it closes 37 per
	//     cent of the discrepancy the simplified transfer cannot touch.
	//   - Higher scattering orders on the scattered term, after Winkler
	//     (2022). Eq. 11's kernel is first order and GAMBONS stops there;
	//     against the published all-sky export our airglow-free sky is 0.046
	//     mag too faint, which is the direction this corrects.
	//   - Moonlight, from ROLO reflectance and single scattering. GAMBONS
	//     models a moonless night and has no term for it at all, which makes
	//     this the largest single capability difference and the reason this
	//     preset cannot be checked against their export.
	//   - Artificial skyglow over a caller's ground-emitter inventory,
	//     Kocifaj (2022). Also absent from GAMBONS, which models the natural
	//     sky alone.
	//   - Pickering (2002) airmass rather than Kasten & Young (1989).
	//     Measured, the two agree to better than three parts in a thousand
	//     above five degrees — no hundredths of a magnitude in any band — and
	//     diverge only in the last degrees, where Pickering is better behaved.
	//
	// # What it costs
	//
	// Reference fidelity, so about three orders of magnitude more per
	// direction than [GAMBONSWeb], and two inputs the others do not need: a
	// solar spectrum for the lunar reflectance and a ground-emitter inventory.
	// A caller without either should use [NaturalSky], which is the same
	// natural sky under the simplified transfer.
	Observatory Preset = "observatory"
)

// Fidelity returns the level [Model.Estimate] must be asked for to evaluate
// this preset as its source defines it.
//
// [GAMBONSFull] needs [Reference], because its scattering term is the
// hemispheric integral and that is the level which runs it. The other two are
// [Standard]: their transfer is already inside the components, and asking for
// Reference would add a scattering term on top of a stand-in for one and count
// it twice.
func (p Preset) Fidelity() (Fidelity, error) {
	switch p {
	case GAMBONSWeb, NaturalSky:
		return Standard, nil
	case GAMBONSFull, Observatory:
		return Reference, nil
	default:
		return 0, fmt.Errorf("%w: unknown preset %q", ErrPreset, p)
	}
}

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
	case GAMBONSFull, Observatory:
		// Not a scattering choice: the absence of one. The full model puts
		// the scattered light in Eq. 11 rather than in an effective depth,
		// so the direct term carries the true extinction.
		return 1, nil
	default:
		return 0, fmt.Errorf("%w: unknown preset %q", ErrPreset, p)
	}
}

// MultipleScattering reports whether this preset wants the higher scattering
// orders a first-order integral misses, for
// [github.com/TuSKan/astrogo/atmosphere.Builder.MultipleScattering].
//
// Only [Observatory]. The three reproductions must not have it: Masana et al.
// (2024) Eq. 11 is explicitly first order, so a scene claiming to be GAMBONS
// and carrying higher orders is no longer reproducing GAMBONS.
//
// Reported for the same reason as [Preset.DiffuseKappa] and [Preset.Fidelity]:
// it belongs to the atmosphere, the scene owns the atmosphere, and a caller
// who has to remember it is a caller who will one day forget it. Forgetting
// this one is quiet — the sky simply comes out a few per cent fainter with
// nothing to say why.
func (p Preset) MultipleScattering() (bool, error) {
	switch p {
	case GAMBONSWeb, NaturalSky, GAMBONSFull:
		return false, nil
	case Observatory:
		return true, nil
	default:
		return false, fmt.Errorf("%w: unknown preset %q", ErrPreset, p)
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

	// SolarSpectrum is the solar spectral irradiance at the 32 ROLO bands,
	// which the lunar reflectance is multiplied by. Required by [Observatory]
	// and unused by the others, since GAMBONS models a moonless night.
	//
	// [github.com/TuSKan/astrogo/skybrightness/dataset/solar] fetches the
	// CALSPEC reference and samples it onto those bands. It is an input rather
	// than a constant because ROLO's absolute scale depends on the choice.
	SolarSpectrum []float64

	// Emitters is the ground-emitter inventory artificial skyglow is computed
	// over. Required by [Observatory].
	//
	// There is no default and there cannot be one: satellite radiance alone
	// cannot determine a source spectrum or an upward emission function, and
	// a preset that invented an inventory would be reporting somebody else's
	// city.
	Emitters []GroundEmitter

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

	components := []Component{
		starlight, galactic, NewExtragalacticBackground(), NewZodiacalLight(), glow,
	}

	// Observatory adds the two terms GAMBONS has no counterpart for. Both need
	// data the natural sky does not, which is why they are the preset's own
	// required inputs rather than optional extras on every preset.
	if p == Observatory {
		moon, err := NewScatteredMoonlight(in.SolarSpectrum)
		if err != nil {
			return nil, fmt.Errorf("%w %q: moonlight: %w", ErrPreset, p, err)
		}

		artificial, err := NewArtificialSkyglow(in.Emitters)
		if err != nil {
			return nil, fmt.Errorf("%w %q: artificial skyglow: %w", ErrPreset, p, err)
		}

		components = append(components, moon, artificial)
	}

	model, err := NewModel(string(p), components...)
	if err != nil {
		return nil, fmt.Errorf("%w %q: %w", ErrPreset, p, err)
	}

	return model, nil
}
