//go:build network

package plan_test

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

// horizonsRangeRate fetches JPL Horizons' own topocentric range-rate
// (quantity 20, the second column of the pair) for one body, at one instant,
// from a site on the equator at the prime meridian.
func horizonsRangeRate(t *testing.T, command, tlistJD string) float64 {
	t.Helper()

	url := fmt.Sprintf("https://ssd.jpl.nasa.gov/api/horizons.api?format=text"+
		"&COMMAND='%s'&CENTER='coord@399'&COORD_TYPE=GEODETIC&SITE_COORD='0,0,0'"+
		"&MAKE_EPHEM='YES'&EPHEM_TYPE='OBSERVER'&QUANTITIES='20'&TLIST='%s'"+
		"&CSV_FORMAT='YES'&OBJ_DATA='NO'", command, tlistJD)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		testutil.SkipOnUpstreamFailure(t, err)
		t.Fatalf("Horizons: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		testutil.SkipOnUpstreamFailure(t, err)
		t.Fatalf("read Horizons: %v", err)
	}

	body := string(raw)

	// Only the block between $$SOE and $$EOE is ephemeris. The header carries
	// comma-bearing lines too, and a "last parseable float on the line"
	// heuristic reads the body's radius out of one of them — 695700 km for
	// the Sun, which is what the first version of this test compared against
	// and how the mistake announced itself.
	var inData bool

	for line := range strings.SplitSeq(body, "\n") {
		trimmed := strings.TrimSpace(line)

		switch trimmed {
		case "$$SOE":
			inData = true

			continue
		case "$$EOE":
			inData = false
		}

		if !inData || !strings.Contains(trimmed, ",") {
			continue
		}

		// QUANTITIES='20' yields the date, the range and the range-rate; the
		// rate is the last number on the row.
		var nums []float64

		for c := range strings.SplitSeq(trimmed, ",") {
			if v, err := strconv.ParseFloat(strings.TrimSpace(c), 64); err == nil {
				nums = append(nums, v)
			}
		}

		if len(nums) >= 2 {
			return nums[len(nums)-1]
		}
	}

	t.Skip("Horizons returned no parseable range-rate")

	return 0
}

// TestMovingBodyRadialVelocityAgainstHorizons checks the computed radial
// velocity against the service that publishes it.
//
// # What the bound is, and what it is not
//
// It is not an accuracy claim about this library's ephemeris. Both sides
// compute the same quantity, but from different ephemerides: Horizons reads
// DE440 and this uses ephemeris.Default(), gofa's truncated analytical
// series. The residual is therefore dominated by the series, not by the
// geometry under test.
//
// So the bound comes from the series' own published velocity error. Plan94's
// documentation gives maximum absolute range-rate differences against DE200
// of 0.9 m/s for Venus, 2.5 for Mars and 8.2 for Jupiter; Epv00 quotes 5.0
// mm/s for the Sun path and Moon98 a comparable figure. Tripled here, which
// covers DE200 against DE440 and leaves the test measuring whether the
// projection, the frame and the diurnal term are right — a sign error in any
// of them is a whole km/s, not a few m/s.
func TestMovingBodyRadialVelocityAgainstHorizons(t *testing.T) {
	testutil.RequireReachable(t, "ssd.jpl.nasa.gov:443")

	site, err := coord.NewGeodetic(angle.Deg(0), angle.Deg(0), 0)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	// A fixed instant, so a failure is reproducible.
	const tlistJD = "2461114.5"

	at := time.FromJD(2461114.5, time.UTC)
	ctx := coord.NewContext(at, site, atmosphere.Refraction{Pressure: 0})
	e := ephemeris.Default()

	for _, tc := range []struct {
		name, command string
		obs           plan.Observable
		toleranceKmS  float64
	}{
		{"Sun", "10", plan.NewSun(e), 0.015},
		{"Moon", "301", plan.NewMoon(e), 0.015},
		{"Venus", "299", plan.NewVenus(e), 0.0027},
		{"Mars", "499", plan.NewMars(e), 0.0075},
		{"Jupiter", "599", plan.NewJupiter(e), 0.0246},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := plan.RadialVelocity(tc.obs, ctx)
			if err != nil {
				t.Fatalf("RadialVelocity: %v", err)
			}

			want := horizonsRangeRate(t, tc.command, tlistJD)

			t.Logf("%s: astrogo %+.4f km/s, Horizons %+.4f, difference %+.1f m/s",
				tc.name, got, want, (got-want)*1e3)

			if math.Abs(got-want) > tc.toleranceKmS {
				t.Errorf("radial velocity %+.4f km/s against Horizons' %+.4f — a difference of "+
					"%.1f m/s, past the %.1f m/s the analytical series' own published velocity "+
					"error allows", got, want, (got-want)*1e3, tc.toleranceKmS*1e3)
			}
		})
	}
}
