package constants_test

import (
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/constants"
)

// TestEphemerisSetIsComplete keeps a member from being added to the struct
// and left at its zero value, which for a mass parameter is a body with no
// gravity — plausible-looking arithmetic all the way to a wrong orbit.
func TestEphemerisSetIsComplete(t *testing.T) {
	all := constants.DE440.All()
	if len(all) != 18 {
		t.Fatalf("DE440.All() has %d members, want 18", len(all))
	}

	for _, c := range all {
		switch {
		case c.Name == "":
			t.Errorf("a member has no Name: %+v", c)
		case c.Symbol == "":
			t.Errorf("%s has no Symbol", c.Name)
		case c.Value <= 0:
			t.Errorf("%s has Value %v; a mass parameter is positive", c.Name, c.Value)
		case c.Reference == "":
			t.Errorf("%s cites no source", c.Name)
		case c.Unit != constants.DE440.SunGravitationalParameter.Unit:
			t.Errorf("%s is in %v, not the set's unit", c.Name, c.Unit)
		}
	}
}

// TestEphemerisSetCitesBothProvenances is not pedantry. NAIF takes the system
// values from DE440's own ASTRO-VALUES file and the per-planet values from the
// natural-satellite release forms, which are fitted separately and updated on
// their own schedule. A reader comparing against a paper needs to know which
// they are looking at.
func TestEphemerisSetCitesBothProvenances(t *testing.T) {
	if ref := constants.DE440.MarsSystemGravitationalParameter.Reference; !strings.Contains(ref, "DE440") {
		t.Errorf("system parameter cites %q, expected the DE440 ephemeris", ref)
	}

	if ref := constants.DE440.MarsGravitationalParameter.Reference; !strings.Contains(ref, "satellite") {
		t.Errorf("planet parameter cites %q, expected the satellite release forms", ref)
	}
}

// TestSystemParameterExceedsBodyParameter is the property the two families
// exist to express, checked offline so a swapped pair fails without a network.
func TestSystemParameterExceedsBodyParameter(t *testing.T) {
	pairs := []struct {
		name           string
		system, planet constants.Constant
	}{
		{"Mars", constants.DE440.MarsSystemGravitationalParameter, constants.DE440.MarsGravitationalParameter},
		{"Jupiter", constants.DE440.JupiterSystemGravitationalParameter, constants.DE440.JupiterGravitationalParameter},
		{"Saturn", constants.DE440.SaturnSystemGravitationalParameter, constants.DE440.SaturnGravitationalParameter},
		{"Uranus", constants.DE440.UranusSystemGravitationalParameter, constants.DE440.UranusGravitationalParameter},
		{"Neptune", constants.DE440.NeptuneSystemGravitationalParameter, constants.DE440.NeptuneGravitationalParameter},
		{"Pluto", constants.DE440.PlutoSystemGravitationalParameter, constants.DE440.PlutoGravitationalParameter},
	}

	for _, p := range pairs {
		if p.system.Value <= p.planet.Value {
			t.Errorf("%s: system %v is not greater than the body's %v — the two may be swapped",
				p.name, p.system.Value, p.planet.Value)
		}
	}
}

// TestPlutoIsTheCaseThatMatters pins the doc comment's own example, and the
// direction of it, because the comment first had that direction backwards.
//
// Charon is massive enough relative to Pluto that the two parameters differ
// by more than a tenth — and because two-body relative motion is governed by
// the sum of the masses, it is the *system* parameter that reproduces
// Charon's published period.
func TestPlutoIsTheCaseThatMatters(t *testing.T) {
	sys := constants.DE440.PlutoSystemGravitationalParameter.Value
	body := constants.DE440.PlutoGravitationalParameter.Value

	excess := (sys - body) / body
	if math.Abs(excess-0.122) > 0.005 {
		t.Errorf("Pluto system exceeds body by %.1f%%, doc comment says 12%%", 100*excess)
	}

	// Charon's period from each, against the published 6.3872 days.
	const (
		aMeters   = 19_596e3
		published = 6.3872
	)

	period := func(gm float64) float64 {
		return 2 * math.Pi * math.Sqrt(aMeters*aMeters*aMeters/gm) / 86400
	}

	if got := period(sys); math.Abs(got-published) > 0.001 {
		t.Errorf("system parameter gives Charon %.4f d, published is %.4f", got, published)
	}

	if got := period(body); math.Abs(got-published) < 0.2 {
		t.Errorf("body parameter gives Charon %.4f d, which is too close to the published "+
			"%.4f — the two parameters should disagree here by about six percent", got, published)
	}
}

// TestSunAgreesWithTheIAUNominalValue is a cross-check between two independent
// tables in this package. IAU 2015 B3 fixes the nominal solar mass parameter
// by convention; DE440 fits one. They are different kinds of number and should
// still agree to the precision B3 quotes, which is 8 significant figures.
func TestSunAgreesWithTheIAUNominalValue(t *testing.T) {
	iau := constants.IAU.SunGravitationalParameter.Value
	de := constants.DE440.SunGravitationalParameter.Value

	if rel := math.Abs(de-iau) / iau; rel > 1e-8 {
		t.Errorf("DE440 Sun GM %.17g and IAU nominal %.17g differ by %.3g", de, iau, rel)
	}
}

// TestEphemerisPointsAtAVintage keeps the alias from being left dangling when
// a new ephemeris is added.
func TestEphemerisPointsAtAVintage(t *testing.T) {
	if constants.Ephemeris.Name() != constants.DE440.Name() {
		t.Errorf("Ephemeris is %q, DE440 is %q", constants.Ephemeris.Name(), constants.DE440.Name())
	}

	if constants.Ephemeris.Name() == "" {
		t.Error("the current ephemeris vintage has no name")
	}
}
