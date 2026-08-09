package natural

import (
	"fmt"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/skybrightness"
)

// FastConfig configures NewFastEngine.
type FastConfig struct {
	// Ephemeris is used for Moon/Sun positions; nil defaults to
	// ephemeris.Default() (fully offline).
	Ephemeris eph.Provider
	// AirglowSB is the constant V-band airglow floor, mag/arcsec^2; 0
	// defaults to DefaultConstantAirglowV.
	AirglowSB float64
	// ExtinctionV is the V-band extinction coefficient, mag/airmass; 0
	// defaults to DefaultMoonExtinctionV.
	ExtinctionV float64
	// Transmission is the engine's atmospheric transmission model; nil
	// means no transmission is applied (EvaluationOptions.
	// ComputeTransmission has nothing to compute against). Most callers
	// wanting a physically meaningful limiting magnitude should supply
	// atmos.NewRayleighOnly() or a later phase's fuller model.
	Transmission skybrightness.TransmissionModel
}

// NewFastEngine builds a skybrightness.Engine running only the two fast,
// simplified components (constant airglow + Krisciunas & Schaefer
// scattered moonlight) in ModeFast — the fastest, fully-offline,
// zero-data-dependency engine this package offers, and the direct
// successor to astrogo v1's default behavior. See docs/skybrightness.md
// §15.
func NewFastEngine(cfg FastConfig) (skybrightness.Engine, error) {
	k := cfg.ExtinctionV
	if k == 0 {
		k = DefaultMoonExtinctionV
	}

	airglow := NewConstantAirglow()
	if cfg.AirglowSB != 0 {
		airglow = NewConstantAirglowSB(cfg.AirglowSB)
	}

	moon := NewKrisciunasSchaeferMoonlight(
		WithMoonProvider(cfg.Ephemeris),
		WithMoonExtinction(k),
	)

	eng, err := skybrightness.NewCompositeEngine(skybrightness.CompositeConfig{
		Name: skybrightness.AlgorithmRef{
			Name: "natural.FastEngine", Version: "1.0.0",
			Citation: "astrogo v1 default sky-brightness behavior, re-implemented (docs/skybrightness.md §15)",
		},
		Components:   []skybrightness.Component{airglow, moon},
		Transmission: cfg.Transmission,
		Mode:         skybrightness.ModeFast,
	})
	if err != nil {
		return nil, fmt.Errorf("natural: NewFastEngine: %w", err)
	}

	return eng, nil
}
