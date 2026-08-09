package natural

import (
	"fmt"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/skybrightness"
)

// LegacyConfig configures NewLegacyEngine.
type LegacyConfig struct {
	// Ephemeris is used for Moon/Sun positions; nil defaults to
	// ephemeris.Default() (fully offline).
	Ephemeris eph.Provider
	// AirglowSB is the constant V-band airglow floor, mag/arcsec^2; 0
	// defaults to DefaultLegacyAirglowV.
	AirglowSB float64
	// ExtinctionV is the V-band extinction coefficient, mag/airmass; 0
	// defaults to DefaultLegacyExtinctionV.
	ExtinctionV float64
}

// NewLegacyEngine builds a skybrightness.Engine running only the two
// Legacy* components (airglow + scattered moonlight) in ModeLegacy — the
// fastest, fully-offline, zero-data-dependency engine this package
// offers, and the direct successor to astrogo v1's default behavior. See
// docs/skybrightness.md §15.
func NewLegacyEngine(cfg LegacyConfig) (skybrightness.Engine, error) {
	k := cfg.ExtinctionV
	if k == 0 {
		k = DefaultLegacyExtinctionV
	}

	airglow := NewLegacyAirglow()
	if cfg.AirglowSB != 0 {
		airglow = NewLegacyAirglowSB(cfg.AirglowSB)
	}

	moon := NewLegacyMoonlight(
		WithLegacyMoonProvider(cfg.Ephemeris),
		WithLegacyMoonExtinction(k),
	)

	eng, err := skybrightness.NewCompositeEngine(skybrightness.CompositeConfig{
		Name: skybrightness.AlgorithmRef{
			Name: "natural.LegacyEngine", Version: "1.0.0",
			Citation: "astrogo v1 default sky-brightness behavior, re-implemented (docs/skybrightness.md §15)",
		},
		Components: []skybrightness.Component{airglow, moon},
		Mode:       skybrightness.ModeLegacy,
	})
	if err != nil {
		return nil, fmt.Errorf("natural: NewLegacyEngine: %w", err)
	}

	return eng, nil
}
