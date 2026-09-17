package gofaext_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/gofaext"
)

// TestStarpvAgainstSOFAsOwnVector checks the wrapper against the vector SOFA
// publishes for iauStarpv in its validation program t_sofa_c.c, reached here
// through gofa's transcription of it.
//
// That is the right oracle for a wrapper: it tests nothing about the astronomy,
// which is SOFA's, and everything about whether the arguments arrive in the
// right order and the results come back in the right slots — which is the only
// thing a wrapper can get wrong, and the thing a plausible-looking answer would
// hide.
func TestStarpvAgainstSOFAsOwnVector(t *testing.T) {
	t.Parallel()

	const (
		ra  = 0.01686756
		dec = -1.093989828
		pmr = -1.78323516e-5
		pmd = 2.336024047e-6
		px  = 0.74723
		rv  = -21.6
	)

	pv, status := gofaext.Starpv(ra, dec, pmr, pmd, px, rv)

	if status != 0 {
		t.Errorf("status = %d, want 0 — SOFA's own case overrides nothing", status)
	}

	for _, tc := range []struct {
		name string
		got  float64
		want float64
		tol  float64
	}{
		{"position x (au)", pv[0][0], 126668.5912743160601, 1e-10},
		{"position y (au)", pv[0][1], 2136.792716839935195, 1e-12},
		{"position z (au)", pv[0][2], -245251.2339876830091, 1e-10},

		{"velocity x (au/day)", pv[1][0], -0.4051854008955659551e-2, 1e-13},
		{"velocity y (au/day)", pv[1][1], -0.6253919754414777970e-2, 1e-15},
		{"velocity z (au/day)", pv[1][2], 0.1189353714588109341e-1, 1e-13},
	} {
		if math.Abs(tc.got-tc.want) > tc.tol {
			t.Errorf("%s = %.17g, want %.17g (tolerance %g)", tc.name, tc.got, tc.want, tc.tol)
		}
	}
}

// TestStarpvReportsWhatItOverrode covers the status, which is the reason this
// wrapper exists at all rather than callers using gofa.Starpv directly.
//
// SOFA's own iauFk52h and iauH2fk5 discard it, and that is precisely what made
// #331 silent: a proper motion zeroed with nothing said. Both astrogo callers
// — fk5hip.go's dispatch and coord.SpaceVelocity's refusal — turn on this
// value, so it needs to mean what it says.
func TestStarpvReportsWhatItOverrode(t *testing.T) {
	t.Parallel()

	const (
		ra  = 2.1
		dec = -0.4
		pm  = 7.27e-7 // about 150 mas/yr in radians per year
	)

	for _, tc := range []struct {
		name string
		px   float64
		pm   float64
		rv   float64
		// SOFA adds 1 for an overridden distance and 2 for a clamped speed.
		wantBits int
	}{
		{"an ordinary star", 0.05, pm, -30, 0},
		{"a distant but representable star", 1e-3, pm, 0, 0},

		// No parallax: the distance is overridden to PXMIN, putting the star
		// at 10 Mpc, where this proper motion is some 24c — so both guards
		// fire and the bits add.
		{"no parallax, with motion", 0, pm, 0, 1 | 2},

		// The same override without the motion to make it exceed VMAX.
		{"no parallax, at rest", 0, 0, 0, 1},

		// At PXMIN exactly the distance stands, but the speed still does not.
		{"at PXMIN, with motion", 1e-7, pm, 0, 2},
	} {
		_, status := gofaext.Starpv(ra, dec, tc.pm, tc.pm, tc.px, tc.rv)

		if status != tc.wantBits {
			t.Errorf("%s (px=%g, pm=%g): status = %d, want %d",
				tc.name, tc.px, tc.pm, status, tc.wantBits)
		}
	}
}
