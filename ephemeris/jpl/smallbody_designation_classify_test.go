package jpl

import (
	"errors"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/core"
)

// TestNumberedAsteroidClassifiesTheDesignation pins the boundary that decides
// whether the substitution guard has a question it can ask at all.
//
// Everything at or above NAIF's numbered-asteroid allocation is some other
// namespace — Horizons' comet index issues SPK-IDs there — and a designation
// naming one of those predicts nothing about which body should arrive.
func TestNumberedAsteroidClassifiesTheDesignation(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		designation string
		want        core.ID
		wantOK      bool
	}{
		{"an asteroid number", "433", core.SmallBodyID(433), true},
		{"the semicolon form Horizons prefers", "433;", core.SmallBodyID(433), true},
		{"the ambiguous bare one", "1", core.SmallBodyID(1), true},
		{"the last number in the block", "999999", core.SmallBodyID(999999), true},

		{"the first number outside it", "1000000", 0, false},
		{"a comet SPK-ID from Horizons", "1000390", 0, false},
		{"another comet SPK-ID", "1003928", 0, false},

		{"zero", "0", 0, false},
		{"negative", "-5", 0, false},
		{"a name", "Ceres", 0, false},
		{"a provisional designation", "2019 M4", 0, false},
		{"nothing", "", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := numberedAsteroid(tc.designation)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("numberedAsteroid(%q) = (%v, %v), want (%v, %v)",
					tc.designation, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

// TestVerifyRequestedBodyLoadedAcceptsACometSPKID is the regression.
//
// Every comet reached by its Horizons SPK-ID failed as ErrWrongSmallBody —
// the loudest and most specific claim this package can make, that Horizons
// substituted a different object — while the body requested was sitting in
// the list. core.SmallBodyID reports 0 for a number outside its block, and
// composing it into the check by hand asked whether the kernel contained
// body 0. It never does, so the guard concluded substitution every time.
//
// Three of these turned up in one run of plan's network suite: C/2002 J5
// (LINEAR), C/2000 A1 (Montani) and C/2019 M4 (TESS).
func TestVerifyRequestedBodyLoadedAcceptsACometSPKID(t *testing.T) {
	t.Parallel()

	const comet = core.ID(1000390) // C/2002 J5 (LINEAR)

	p := &Provider{kernel: "1000390"}

	if err := p.verifyRequestedBodyLoaded([]core.ID{core.Earth, core.Sun, comet}); err != nil {
		t.Fatalf("a comet that loaded correctly was rejected: %v", err)
	}
}

// TestVerifyRequestedBodyLoadedStillCatchesTheSubstitution is the half that
// must not be lost to the fix.
//
// Asking Horizons for "1" returns comet 1000036 rather than 1 Ceres, and
// catching that is the whole reason this check exists.
func TestVerifyRequestedBodyLoadedStillCatchesTheSubstitution(t *testing.T) {
	t.Parallel()

	const whatHorizonsActuallySends = core.ID(1000036)

	p := &Provider{kernel: "1"}

	err := p.verifyRequestedBodyLoaded([]core.ID{core.Earth, core.Sun, whatHorizonsActuallySends})
	if !errors.Is(err, ErrWrongSmallBody) {
		t.Fatalf("verifyRequestedBodyLoaded = %v, want ErrWrongSmallBody", err)
	}

	// The message has to name the body that was wanted, or a reader is told
	// a substitution happened without being told what to look for.
	if !strings.Contains(err.Error(), core.SmallBodyID(1).String()) {
		t.Errorf("error does not name the requested body: %v", err)
	}
}

// TestVerifyRequestedBodyLoadedAcceptsTheAsteroidItAskedFor keeps the ordinary
// case honest: a numbered asteroid that arrives is not an error.
func TestVerifyRequestedBodyLoadedAcceptsTheAsteroidItAskedFor(t *testing.T) {
	t.Parallel()

	for _, designation := range []string{"433", "433;"} {
		t.Run(designation, func(t *testing.T) {
			t.Parallel()

			p := &Provider{kernel: designation}

			loaded := []core.ID{core.Earth, core.Sun, core.SmallBodyID(433)}
			if err := p.verifyRequestedBodyLoaded(loaded); err != nil {
				t.Fatalf("%q: %v", designation, err)
			}
		})
	}
}
