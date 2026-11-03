//go:build network

package skybrightness_test

import (
	"context"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/skybrightness/dataset/passband"
	"github.com/TuSKan/astrogo/skybrightness/dataset/solar"
	"github.com/TuSKan/astrogo/time"
)

// ks91Scatter is the rms scatter Krisciunas & Schaefer (1991) report for their
// own fit to the Mauna Kea moonlight data, 23 per cent. It bounds how far a
// single row can sit from a correct model through what the measurement and
// the night left out, so it is the data's share of the bound below.
const ks91Scatter = 0.23

// TestScatteredMoonlightAgainstKS91WithCALSPEC holds the component to Krisciunas
// & Schaefer (1991) Table 1 on the CALSPEC solar spectrum, projected through
// the Bessell V band, which is what makes it a measurement of the component
// rather than of a fixture. It is the comparison docs/VALIDATION.md quotes.
//
// # What it asserts
//
// The mean of ln(model / measured) over the ten rows inside ROLO's phase range
// must lie within the component's own uncertainty, averaged over those rows,
// combined in quadrature with the data's: ks91Scatter over the square root of
// the row count. That is the budget's claim, 16-18% in V, put to the one
// published measurement with its geometry stated. Each row must also stay
// within the two magnitudes the offline test holds, which catches a factor of
// ten anywhere.
//
// # What it measured
//
// Before #938 the mean was 0.30 mag faint, outside the budget. Two terms
// carried half of it: ROLO's albedo, which Velikodsky et al. (2011) find 13
// per cent faint and which the component now corrects as Jones et al. (2013)
// do, and reading one 554 nm sample in place of the V band, worth 0.02 mag.
//
// What remains follows the Moon's altitude in a way single scattering cannot.
// At 68-69 degrees from the Moon, the measured sky brightens 1.6 times as the
// Moon rises from 82 to 71 degrees of zenith distance on 1991 January 4, where
// single scattering at the night's measured extinction gives 1.21: the
// plane-parallel column integral, worked by hand, not a property of this code.
// Extinction along the Moon's low beam above the night's zenith figure, haze
// at the inversion below the site, would do it, and one coefficient cannot
// describe that.
func TestScatteredMoonlightAgainstKS91WithCALSPEC(t *testing.T) {
	testutil.RequireReachable(t, "archive.stsci.edu:443")
	testutil.RequireReachable(t, "svo2.cab.inta-csic.es:443")

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// The file is a few megabytes; consent is still required.
	remote.EnableDownloads(64<<20, remote.CALSPEC)
	defer remote.EnableDownloads(0)

	m, err := solar.NewScatteredMoonlight(ctx)
	if err != nil {
		testutil.SkipOnUpstreamFailure(t, err)
		t.Fatalf("NewScatteredMoonlight: %v", err)
	}

	v, err := passband.Fetch(ctx, "Generic/Bessell.V")
	if err != nil {
		testutil.SkipOnUpstreamFailure(t, err)
		t.Fatalf("passband.Fetch: %v", err)
	}

	grid := skybrightness.DefaultOpticalGrid()

	var sum, sumSq, budget float64

	n := 0

	for _, o := range ks91Table1 {
		if !ks91InROLODomain(o) {
			continue
		}

		k, _ := ks91Night(o)

		scene := &skybrightness.Scene{
			Observer:   ks91Site(t),
			Time:       o.when,
			Atmosphere: ks91Air(t, k),
			Ephemeris:  eph.Default(),
		}

		dst := skybrightness.NewSpectralRadiance(grid)

		contribution, err := m.AddRadiance(ctx, dst, grid, coord.NewAltAz(angle.Deg(90-o.skyZ), angle.Deg(o.skyAz)), scene)
		if err != nil {
			t.Fatalf("%v: AddRadiance: %v", o.when, err)
		}

		if !contribution.Known {
			t.Fatalf("%v: the component reports no uncertainty", o.when)
		}

		vMag, err := magnitude.SurfaceBrightness(dst, grid, v, magnitude.Vega, 0.9)
		if err != nil {
			t.Fatalf("%v: SurfaceBrightness: %v", o.when, err)
		}

		moon := ks91NanoLamberts(vMag)
		dmag := -2.5 * math.Log10(moon/o.bMoon)

		t.Logf("%v: rho %v, Moon Z %v: %.0f nL against %.0f measured, ratio %.3f (%+.2f mag), budget %.1f%%",
			o.when, o.rho, o.moonZ, moon, o.bMoon, moon/o.bMoon, dmag, 100*contribution.Relative)

		if math.Abs(dmag) > 2 {
			t.Errorf("%v: %.0f nL against %.0f measured, %+.2f mag", o.when, moon, o.bMoon, dmag)
		}

		l := math.Log(moon / o.bMoon)
		sum += l
		sumSq += l * l
		budget += contribution.Relative
		n++
	}

	mean := sum / float64(n)
	scatter := math.Sqrt((sumSq - float64(n)*mean*mean) / float64(n-1))
	bound := math.Hypot(budget/float64(n), ks91Scatter/math.Sqrt(float64(n)))

	t.Logf("%d rows: mean ratio %.3f (%+.2f mag), scatter %.2f in ln ratio, bound %.3f",
		n, math.Exp(mean), -2.5*mean/math.Ln10, scatter, bound)

	if math.Abs(mean) > bound {
		t.Errorf("the mean ln ratio is %+.3f, outside the %.3f the component's budget and the data allow",
			mean, bound)
	}
}
