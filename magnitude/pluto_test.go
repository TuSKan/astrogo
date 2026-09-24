package magnitude_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/time"
)

// hubbleVisit is one of Buie et al.'s (2010) twelve visit-averaged V
// measurements, as Tables 3 (Pluto) and 5 (Charon) give them: Julian Date,
// phase angle, Pluto's sub-Earth east longitude in the paper's system, and
// each body's V at mean opposition distance with no phase correction.
type hubbleVisit struct {
	jd, phase, lon, pluto, charon float64
}

var hubbleVisits = []hubbleVisit{
	{2452436.862716, 0.361, 358.75, 15.3460, 17.2993},
	{2452787.613817, 0.513, 27.75, 15.3909, 17.3432},
	{2452550.702273, 1.712, 62.68, 15.4945, 17.4266},
	{2452799.274918, 0.316, 90.63, 15.4712, 17.2270},
	{2452472.996286, 1.238, 122.50, 15.4924, 17.3429},
	{2452772.634032, 0.911, 151.89, 15.3719, 17.3156},
	{2452440.068009, 0.411, 178.12, 15.2808, 17.2341},
	{2452688.556350, 1.734, 210.66, 15.2690, 17.3823},
	{2452458.097114, 0.859, 242.13, 15.2661, 17.3168},
	{2452789.676579, 0.463, 271.51, 15.2976, 17.2751},
	{2452444.277588, 0.500, 300.90, 15.3213, 17.2916},
	{2452750.264898, 1.442, 332.49, 15.3632, 17.4330},
}

// TestPlutoCharonReproducesTheHubblePhotometry evaluates the model at each of
// the paper's twelve visits — its own phase angle and longitude, at the mean
// opposition distance the photometry is reduced to — and compares it with
// what Hubble measured. This is the model's validation: the light-curve
// coefficients come from the paper's tables and the phase curve from its
// Hapke parameters, so agreement here is the implementation agreeing with the
// data those were fitted to.
//
// Measured: mean |O−C| 0.0043 mag for Pluto and 0.0050 for Charon, 0.014 at
// worst, against the paper's own fit residual of 0.013.
func TestPlutoCharonReproducesTheHubblePhotometry(t *testing.T) {
	const (
		tolVisit = 0.02
		tolMean  = 0.006
	)

	var sumPluto, sumCharon float64

	for _, v := range hubbleVisits {
		got := magnitude.PlutoCharonMagnitudesAt(39.5, 38.5, v.phase, v.lon)

		dp, dc := got.Pluto-v.pluto, got.Charon-v.charon
		sumPluto += math.Abs(dp)
		sumCharon += math.Abs(dc)

		if math.Abs(dp) > tolVisit || math.Abs(dc) > tolVisit {
			t.Errorf("JD %.3f (phase %.3f°, longitude %.2f°): Pluto %.4f vs %.4f (%+.4f), Charon %.4f vs %.4f (%+.4f)",
				v.jd, v.phase, v.lon, got.Pluto, v.pluto, dp, got.Charon, v.charon, dc)
		}
	}

	n := float64(len(hubbleVisits))
	if sumPluto/n > tolMean || sumCharon/n > tolMean {
		t.Errorf("mean |O−C|: Pluto %.4f, Charon %.4f; limit %.3f", sumPluto/n, sumCharon/n, tolMean)
	}
}

// TestPlutoCharonAddsFluxes: the pair's magnitude is their fluxes added, not
// their magnitudes. At the paper's first visit Hubble measured Pluto at
// 15.3460 and Charon at 17.2993, which together are 15.180; the historical
// law gives 14.915 at that geometry.
func TestPlutoCharonAddsFluxes(t *testing.T) {
	if got := magnitude.AddMagnitudes(15.3460, 17.2993); math.Abs(got-15.180) > 0.001 {
		t.Errorf("Pluto 15.3460 with Charon 17.2993: %.4f, want 15.180", got)
	}

	for _, v := range hubbleVisits {
		m := magnitude.PlutoCharonMagnitudesAt(39.5, 38.5, v.phase, v.lon)

		if got := magnitude.AddMagnitudes(m.Pluto, m.Charon); got != m.Combined {
			t.Fatalf("Combined %v is not Pluto %v with Charon %v (%v)", m.Combined, m.Pluto, m.Charon, got)
		}

		if m.Combined >= m.Pluto || m.Combined >= m.Charon {
			t.Fatalf("Combined %v is not brighter than both Pluto %v and Charon %v", m.Combined, m.Pluto, m.Charon)
		}
	}
}

// TestPlutoCharonGeometryMatchesThePaper computes the geometry
// PlutoCharonApparent evaluates the model at — phase angle and Pluto's
// sub-Earth longitude — at each visit's time, from astrogo's own ephemeris,
// and checks it against what the paper tabulates. The paper's longitude
// system differs from the IAU model by a constant this was calibrated with,
// so what the test checks is that the constant holds at every visit and that
// light time, which moves the longitude by about 10° at Pluto's distance, is
// applied.
//
// Measured: longitude within 0.10° at every visit, the spread of the
// tabulation itself; phase angle to the tabulation's 0.001°.
func TestPlutoCharonGeometryMatchesThePaper(t *testing.T) {
	const (
		tolLongitude = 0.15
		tolPhase     = 0.005
	)

	p := defaultProvider()

	for _, v := range hubbleVisits {
		_, _, phase, lon, err := magnitude.PlutoCharonGeometry(p, time.FromJD(v.jd, time.UTC))
		if err != nil {
			t.Fatalf("JD %.3f: %v", v.jd, err)
		}

		if diff := angle.Deg(lon - v.lon).Wrap180().Degrees(); math.Abs(diff) > tolLongitude {
			t.Errorf("JD %.3f: longitude %.2f°, the paper gives %.2f° (off by %+.2f°)", v.jd, lon, v.lon, diff)
		}

		if diff := phase - v.phase; math.Abs(diff) > tolPhase {
			t.Errorf("JD %.3f: phase angle %.3f°, the paper gives %.3f° (off by %+.3f°)", v.jd, phase, v.phase, diff)
		}
	}
}

// TestPlutoCharonPhaseCurves checks the Hapke phase curves' shape: zero at
// zero phase, rising over the phase angles Earth sees, and Charon's sharp
// opposition surge against Pluto's gentle one, which is what makes a linear
// law impossible for Charon.
//
// It also bounds the one disagreement with the paper that is understood only
// as a size: at 1° the paper's captions give +0.0398 (Pluto) and +0.2549
// (Charon), and this implementation of its parameters gives +0.0438 and
// +0.2636 (see hapkeSphere.at). The photometry test above is the one that
// decides whether that matters; this keeps it from growing unnoticed.
func TestPlutoCharonPhaseCurves(t *testing.T) {
	for _, c := range []struct {
		name      string
		curve     func(float64) float64
		paperAt1  float64
		tolAt1    float64
		minAt03   float64
		maxAt03   float64
		atMaxSeen float64
	}{
		{"Pluto", magnitude.PlutoPhaseCurve, 0.0398, 0.005, 0.005, 0.02, 1.9},
		{"Charon", magnitude.CharonPhaseCurve, 0.2549, 0.010, 0.12, 0.16, 1.9},
	} {
		if got := c.curve(0); got != 0 {
			t.Errorf("%s at 0°: %v, want 0", c.name, got)
		}

		prev := 0.0

		for g := 0.1; g <= c.atMaxSeen; g += 0.1 {
			if got := c.curve(g); got <= prev {
				t.Errorf("%s: %.4f at %.1f° is not above %.4f at %.1f°", c.name, got, g, prev, g-0.1)
			} else {
				prev = got
			}
		}

		if got := c.curve(1); math.Abs(got-c.paperAt1) > c.tolAt1 {
			t.Errorf("%s at 1°: %+.4f, the paper gives %+.4f (limit %.3f)", c.name, got, c.paperAt1, c.tolAt1)
		}

		if got := c.curve(0.3); got < c.minAt03 || got > c.maxAt03 {
			t.Errorf("%s at 0.3°: %+.4f, want within [%.2f, %.2f]", c.name, got, c.minAt03, c.maxAt03)
		}
	}
}

// TestPlutoCharonDisagreesWithTheHistoricalLawAsDocumented keeps
// PlutoCharonApparent's doc comment honest: in the 2020s, far outside the
// sub-Earth latitude it was calibrated at, the model comes out 0.16–0.36 mag
// fainter than PlanetApparent's historical law. Which is nearer Pluto's real
// brightness is not established; this pins the size of the disagreement the
// documentation reports, not either side of it.
func TestPlutoCharonDisagreesWithTheHistoricalLawAsDocumented(t *testing.T) {
	p := defaultProvider()

	for _, d := range []time.Time{
		time.Date(2020, 1, 1, 0, 0, 0, 0, time.LocationUTC),
		time.Date(2022, 4, 1, 0, 0, 0, 0, time.LocationUTC),
		time.Date(2024, 7, 1, 0, 0, 0, 0, time.LocationUTC),
		time.Date(2026, 9, 23, 0, 0, 0, 0, time.LocationUTC),
		time.Date(2026, 12, 20, 0, 0, 0, 0, time.LocationUTC),
	} {
		buie, err := magnitude.PlutoCharonApparent(p, d)
		if err != nil {
			t.Fatalf("%v: %v", d, err)
		}

		historical, err := magnitude.PlanetApparent(p, eph.Pluto, d)
		if err != nil {
			t.Fatalf("%v: %v", d, err)
		}

		if diff := buie.Combined - historical; diff < 0.15 || diff > 0.37 {
			t.Errorf("%v: Buie et al. %.3f, historical law %.3f, difference %+.3f; the documentation says +0.16 to +0.36",
				d, buie.Combined, historical, diff)
		}
	}
}

var errNoState = errors.New("stateless provider")

type statelessProvider struct{}

func (statelessProvider) State(eph.ID, time.Time) (core.State, error) {
	return core.State{}, errNoState
}

func (statelessProvider) Close() error { return nil }

// TestPlutoCharonApparentReturnsTheProviderError: no position, no magnitude,
// and the cause is kept.
func TestPlutoCharonApparentReturnsTheProviderError(t *testing.T) {
	if _, err := magnitude.PlutoCharonApparent(statelessProvider{}, time.J2000()); !errors.Is(err, errNoState) {
		t.Errorf("err = %v, want it to wrap the provider's error", err)
	}
}
