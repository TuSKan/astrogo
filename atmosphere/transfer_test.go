package atmosphere_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/unit"
)

// The optically thin limit is the independent check on the whole
// expression: as the column thins, single-scattered radiance must approach
// E·p·τ_sca·M_v, the textbook result obtained without any path integral at
// all. Getting the prefactor, the scattering fraction or the airmass factor
// wrong all break this, and none of them are visible in the ratio tests.
func TestSingleScatteredRadianceOpticallyThinLimit(t *testing.T) {
	t.Parallel()

	const (
		e  = 2.5
		p  = 0.08
		ms = 1.7
		mv = 2.4
	)

	// Scattering keeps a fixed share of an ever-thinner column.
	for _, tau := range []float64{1e-2, 1e-4, 1e-6, 1e-8} {
		scattering := 0.6 * tau

		got, err := atmosphere.SingleScatteredRadiance(e, p,
			unit.OpticalDepth(scattering), unit.OpticalDepth(tau), ms, mv)
		if err != nil {
			t.Fatalf("SingleScatteredRadiance(tau=%v): %v", tau, err)
		}

		want := e * p * scattering * mv

		// The leading correction is O(tau), so the tolerance tracks tau.
		if rel := math.Abs(got-want) / want; rel > 3*tau {
			t.Errorf("tau = %g: got %.10g, want %.10g (relative %.3e)", tau, got, want, rel)
		}
	}
}

// At equal airmasses the quotient is a removable singularity. It must be
// smooth through that point, not merely defined at it: a bare difference of
// two nearly equal exponentials cancels catastrophically, which would show
// up as noise in the sky exactly where the line of sight passes near the
// source.
func TestSingleScatteredRadianceSingularityIsSmooth(t *testing.T) {
	t.Parallel()

	const (
		e   = 1.0
		p   = 0.1
		tau = 0.3
		m   = 2.0
	)

	at := func(mv float64) float64 {
		v, err := atmosphere.SingleScatteredRadiance(e, p, 0.2, tau, m, mv)
		if err != nil {
			t.Fatalf("SingleScatteredRadiance(M_v=%v): %v", mv, err)
		}

		return v
	}

	exact := at(m)

	// The closed form at M_v = M_s is E·p·(τ_sca/τ)·M_v·τ·e^{−τ·M_s}.
	want := e * p * (0.2 / tau) * m * tau * math.Exp(-tau*m)
	if rel := math.Abs(exact-want) / want; rel > 1e-14 {
		t.Errorf("at equal airmass got %.12g, want %.12g", exact, want)
	}

	for _, eps := range []float64{1e-1, 1e-3, 1e-6, 1e-9, 1e-12} {
		for _, side := range []float64{-1, 1} {
			v := at(m + side*eps)

			if math.IsNaN(v) || v <= 0 {
				t.Fatalf("M_v = M_s %+g gave %v", side*eps, v)
			}

			if rel := math.Abs(v-exact) / exact; rel > 10*eps+1e-12 {
				t.Errorf("M_v = M_s %+g: relative departure %.3e is too large for a smooth function",
					side*eps, rel)
			}
		}
	}
}

// A thicker scattering column scatters more light, but only up to a point:
// beyond an optical depth of order 1 the extinction along both legs wins and
// the observed radiance turns over. Both halves of that are physical, and a
// model that grows without bound would be wrong.
func TestSingleScatteredRadianceSaturates(t *testing.T) {
	t.Parallel()

	const (
		e  = 1.0
		p  = 0.1
		ms = 2.0
		mv = 1.0
	)

	var peak, peakTau float64

	for i := 1; i <= 400; i++ {
		tau := float64(i) / 100

		got, err := atmosphere.SingleScatteredRadiance(e, p,
			unit.OpticalDepth(tau), unit.OpticalDepth(tau), ms, mv)
		if err != nil {
			t.Fatalf("SingleScatteredRadiance(tau=%v): %v", tau, err)
		}

		if got > peak {
			peak, peakTau = got, tau
		}
	}

	if peakTau < 0.2 || peakTau > 3 {
		t.Errorf("radiance peaks at tau = %v, expected an order-unity optical depth", peakTau)
	}

	// Far past the peak the atmosphere is opaque and the source is hidden.
	thick, err := atmosphere.SingleScatteredRadiance(e, p, 20, 20, ms, mv)
	if err != nil {
		t.Fatalf("SingleScatteredRadiance: %v", err)
	}

	if thick >= peak/10 {
		t.Errorf("an opaque column gives %.5g against a peak of %.5g; extinction is not winning", thick, peak)
	}
}

// Linear in the source irradiance, in the phase function and in the
// scattering share — which is what lets a spectrum be evaluated band by band
// and several sources summed.
func TestSingleScatteredRadianceIsLinear(t *testing.T) {
	t.Parallel()

	base, err := atmosphere.SingleScatteredRadiance(1, 0.1, 0.2, 0.4, 2, 1.5)
	if err != nil {
		t.Fatalf("SingleScatteredRadiance: %v", err)
	}

	cases := []struct {
		name          string
		e, p          float64
		sca, ext      unit.OpticalDepth
		wantMultiplex float64
	}{
		{"irradiance", 3, 0.1, 0.2, 0.4, 3},
		{"phase function", 1, 0.3, 0.2, 0.4, 3},
		{"scattering share", 1, 0.1, 0.4, 0.4, 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := atmosphere.SingleScatteredRadiance(tc.e, tc.p, tc.sca, tc.ext, 2, 1.5)
			if err != nil {
				t.Fatalf("SingleScatteredRadiance: %v", err)
			}

			want := base * tc.wantMultiplex
			if rel := math.Abs(got-want) / want; rel > 1e-12 {
				t.Errorf("got %.10g, want %.10g", got, want)
			}
		})
	}
}

// No source, no scattered light — the one case where an exact zero is the
// only acceptable answer.
func TestSingleScatteredRadianceZeroSource(t *testing.T) {
	t.Parallel()

	got, err := atmosphere.SingleScatteredRadiance(0, 0.1, 0.2, 0.4, 2, 1.5)
	if err != nil {
		t.Fatalf("SingleScatteredRadiance: %v", err)
	}

	if got != 0 {
		t.Errorf("got %v, want exactly 0", got)
	}
}

func TestSingleScatteredRadianceRejectsBadInput(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		e, p     float64
		sca, ext unit.OpticalDepth
		ms, mv   float64
		want     error
	}{
		{"negative irradiance", -1, 0.1, 0.2, 0.4, 2, 1.5, atmosphere.ErrSourceIrradiance},
		{"negative scattering", 1, 0.1, -0.1, 0.4, 2, 1.5, atmosphere.ErrOpticalDepth},
		{"zero extinction", 1, 0.1, 0, 0, 2, 1.5, atmosphere.ErrOpticalDepth},
		{"scattering exceeds extinction", 1, 0.1, 0.5, 0.4, 2, 1.5, atmosphere.ErrOpticalDepth},
		{"source airmass below one", 1, 0.1, 0.2, 0.4, 0.5, 1.5, atmosphere.ErrAirmassRange},
		{"view airmass below one", 1, 0.1, 0.2, 0.4, 2, 0, atmosphere.ErrAirmassRange},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := atmosphere.SingleScatteredRadiance(tc.e, tc.p, tc.sca, tc.ext, tc.ms, tc.mv)
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func BenchmarkSingleScatteredRadiance(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if _, err := atmosphere.SingleScatteredRadiance(1, 0.1, 0.2, 0.4, 2, 1.5); err != nil {
			b.Fatal(err)
		}
	}
}
