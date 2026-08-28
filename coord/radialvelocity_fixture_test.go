package coord_test

import (
	"math"
	gotime "time"

	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
)

// rvFixtureCase is one cross-implementation reference point: a
// (site, epoch, target) triple and the barycentric/heliocentric RV
// correction Astropy's SkyCoord.radial_velocity_correction computes for
// it, in km/s.
type rvFixtureCase struct {
	name                    string
	lonDeg, latDeg, heightM float64
	year                    int
	month                   gotime.Month
	day, hour, minute       int
	raDeg, decDeg           float64
	wantBarycentric         float64
	wantHeliocentric        float64
}

// rvFixtures is the cross-implementation reference table for
// BarycentricRVCorrection/HeliocentricRVCorrection, meant to be filled in
// with real output from Astropy's SkyCoord.radial_velocity_correction —
// see the snippet already shared in chat. Left empty until the user
// supplies real reference values: fabricating placeholder numbers here
// would silently validate nothing.
var rvFixtures = []rvFixtureCase{}

// TestRVCorrection_AgainstAstropyFixture cross-checks
// BarycentricRVCorrection/HeliocentricRVCorrection against real Astropy
// output. Skipped while rvFixtures is empty — populate it with real
// (site, epoch, target) -> correction values from Astropy before
// removing the skip.
func TestRVCorrection_AgainstAstropyFixture(t *testing.T) {
	if len(rvFixtures) == 0 {
		t.Skip("Astropy reference fixture not yet supplied")
	}

	for _, tc := range rvFixtures {
		t.Run(tc.name, func(t *testing.T) {
			site, err := coord.NewGeodetic(angle.Deg(tc.lonDeg), angle.Deg(tc.latDeg), tc.heightM)
			testutil.AssertNoError(t, err)

			epoch := time.Date(tc.year, tc.month, tc.day, tc.hour, tc.minute, 0, 0, time.LocationUTC)

			ctx := coord.NewContext(epoch, site, atmosphere.Refraction{Pressure: 0})
			target := coord.NewICRS(angle.Deg(tc.raDeg), angle.Deg(tc.decDeg))

			bary := ctx.BarycentricRVCorrection(target)
			if math.Abs(bary-tc.wantBarycentric) > 0.001 {
				t.Errorf("BarycentricRVCorrection = %v km/s, want %v (Astropy)", bary, tc.wantBarycentric)
			}

			helio, err := ctx.HeliocentricRVCorrection(target)
			testutil.AssertNoError(t, err)

			if math.Abs(helio-tc.wantHeliocentric) > 0.001 {
				t.Errorf("HeliocentricRVCorrection = %v km/s, want %v (Astropy)", helio, tc.wantHeliocentric)
			}
		})
	}
}
