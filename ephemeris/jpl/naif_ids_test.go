//go:build integration

package jpl_test

import (
	"context"
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/constants"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/time"
)

// TestKernelAnswersTheNAIFCodeAskedFor is #809 against de440s: an ID is the
// NAIF code the kernel's chains are indexed by, so the Pluto barycenter is
// served as itself, Pluto's own center — which a planetary kernel does not
// hold — is refused as unknown rather than "no coverage", and the Solar
// System barycenter, which used to be refused, is the origin of every chain.
func TestKernelAnswersTheNAIFCodeAskedFor(t *testing.T) {
	t.Parallel()

	p, err := jpl.NewProvider(context.Background(), core.Planets, "de440s")
	if err != nil {
		t.Fatalf("de440s: %v", err)
	}

	t.Cleanup(func() { _ = p.Close() })

	at := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.LocationUTC)

	// #809's measurement: 34.93 AU.
	pluto, err := eph.At(p, at).Geometric(core.PlutoBarycenter, core.Earth)
	if err != nil {
		t.Fatalf("PlutoBarycenter: %v", err)
	}

	if r := pluto.Pos.Norm(); r < 34.9 || r > 35.0 {
		t.Errorf("PlutoBarycenter at %.3f AU, want 34.93", r)
	}

	for _, center := range []core.ID{core.Pluto, core.Mars, core.Jupiter} {
		_, err := eph.At(p, at).Geometric(center, core.Earth)
		if !errors.Is(err, jpl.ErrUnknownTarget) || errors.Is(err, jpl.ErrNoSegment) {
			t.Errorf("%v: err = %v, want ErrUnknownTarget alone — de440s holds the barycenter, not the planet", center, err)
		}
	}

	// The Solar System barycenter is answered, and where the Sun's wobble
	// puts it: within about 2.2 solar radii, 0.0102 AU, of the Sun's center.
	ssb, err := eph.At(p, at).Geometric(core.SolarSystemBarycenter, core.Earth)
	if err != nil {
		t.Fatalf("SolarSystemBarycenter: %v", err)
	}

	sun, err := eph.At(p, at).Geometric(core.Sun, core.Earth)
	if err != nil {
		t.Fatalf("Sun: %v", err)
	}

	if d := ssb.Pos.Sub(sun.Pos).Norm(); d > 0.0102 {
		t.Errorf("the barycenter is %.4f AU from the Sun, want within 0.0102", d)
	}
}

// TestKernelEarthMoonBarycenterIsTheMoonsShare: DE places the Earth and the
// Moon about their barycenter by their masses, so from the geocentre the
// barycenter is the Moon's vector times GM_Moon / GM_EMB — the definition
// eph.Default answers it by (#809). Held to rounding in km.
func TestKernelEarthMoonBarycenterIsTheMoonsShare(t *testing.T) {
	t.Parallel()

	p, err := jpl.NewProvider(context.Background(), core.Planets, "de440s")
	if err != nil {
		t.Fatalf("de440s: %v", err)
	}

	t.Cleanup(func() { _ = p.Close() })

	mu := constants.Ephemeris.MoonGravitationalParameter.Value /
		constants.Ephemeris.EarthMoonGravitationalParameter.Value

	const kmPerAU = 149597870.7

	for _, at := range []time.Time{
		time.Date(1950, time.January, 1, 0, 0, 0, 0, time.LocationUTC),
		time.Date(2026, time.June, 1, 0, 0, 0, 0, time.LocationUTC),
		time.Date(2100, time.March, 3, 0, 0, 0, 0, time.LocationUTC),
	} {
		emb, err := eph.At(p, at).Geometric(core.EarthMoonBarycenter, core.Earth)
		if err != nil {
			t.Fatalf("%v: %v", at, err)
		}

		moon, err := eph.At(p, at).Geometric(core.Moon, core.Earth)
		if err != nil {
			t.Fatalf("%v: Moon: %v", at, err)
		}

		if d := emb.Pos.Sub(moon.Pos.MulScalar(mu)).Norm() * kmPerAU; d > 1e-6 {
			t.Errorf("%v: the barycenter is %.3g km from the Moon's share of its vector", at, d)
		}
	}
}
