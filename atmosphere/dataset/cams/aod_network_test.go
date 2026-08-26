//go:build network

package cams_test

import (
	"context"
	"math"
	"testing"
	gotime "time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere/dataset/cams"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	_ "github.com/TuSKan/astrogo/remote/s3"
)

// A date the archive is known to hold, used by every test here so they share
// one cached file rather than fetching one each.
var aodEpoch = gotime.Date(2023, 1, 1, 3, 0, 0, 0, gotime.UTC)

func siteAt(tb testing.TB, lonDeg, latDeg float64) *coord.Geodetic {
	tb.Helper()

	g, err := coord.NewGeodetic(angle.Deg(lonDeg), angle.Deg(latDeg), 0)
	if err != nil {
		tb.Fatalf("NewGeodetic: %v", err)
	}

	return g
}

func aodAt(tb testing.TB, lonDeg, latDeg float64) float64 {
	tb.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*gotime.Minute)
	defer cancel()

	v, err := cams.AOD550(ctx, siteAt(tb, lonDeg, latDeg), aodEpoch)
	if err != nil {
		tb.Skipf("CAMS did not answer: %v", err)
	}

	return v
}

// The grid is oriented the way the reader assumes it is.
//
// # Why geography and not a fixture
//
// Nothing in the file says which way latitude runs. The reader takes the
// ECMWF convention — latitude descending from +90, longitude ascending from 0
// — and an assumption like that is exactly the kind that returns plausible
// numbers from the wrong place. A flipped hemisphere or a half-globe
// longitude offset still hands back a perfectly ordinary optical depth.
//
// So this checks the map against the world, at places whose January contrast
// is large and well known: the Indo-Gangetic Plain and eastern China are in
// their haze season, and the Antarctic plateau and the central Pacific are
// about as clean as the atmosphere gets. Any reflection or rotation of the
// grid breaks that ordering.
//
// # How the orientation was established in the first place
//
// By printing the whole field as a coarse character map and looking at it.
// The January maxima landed on equatorial Africa, northern India and eastern
// China, and both poles came out clean — which is the world, and could not
// survive an inverted latitude or a shifted longitude.
//
// That exercise also corrected this test. It first compared the Bodele
// Depression against the Southern Ocean and passed by three per cent, which
// is luck rather than evidence: in January the Saharan dust maximum has moved
// south into the Sahel, and the Southern Ocean carries a genuinely high
// sea-salt load from its own whitecaps. Both endpoints were poor choices,
// and the test would have passed just as happily with a subtly wrong grid.
func TestAODOrientationIsSane(t *testing.T) {
	testutil.RequireReachable(t, "eodata.dataspace.copernicus.eu:443")

	remote.EnableDownloads(50<<20, remote.CopernicusEODATA)

	hazy := []struct {
		name     string
		lon, lat float64
	}{
		{"Indo-Gangetic Plain", 82, 26},
		{"eastern China", 116, 34},
	}

	clean := []struct {
		name     string
		lon, lat float64
	}{
		{"Antarctic plateau", 0, -80},
		{"central Pacific", -140, 0},
	}

	worstHazy := math.Inf(1)

	for _, c := range hazy {
		v := aodAt(t, c.lon, c.lat)
		t.Logf("%-20s AOD550 = %.4f", c.name, v)

		worstHazy = math.Min(worstHazy, v)
	}

	worstClean := 0.0

	for _, c := range clean {
		v := aodAt(t, c.lon, c.lat)
		t.Logf("%-20s AOD550 = %.4f", c.name, v)

		worstClean = math.Max(worstClean, v)

		if v < 0 || v > 5 {
			t.Errorf("%s reads %.4f, which is not a plausible optical depth", c.name, v)
		}
	}

	// A wide margin, because the point is the ordering and not the values.
	// Measured, the gap is nearer tenfold; requiring three catches every
	// reflection of the grid while surviving a different day's weather.
	if worstHazy < 3*worstClean {
		t.Errorf("the haziest of the polluted sites reads %.4f and the least clean of the "+
			"remote ones %.4f; a January haze belt cannot be within three times the "+
			"Antarctic plateau, so the grid is being indexed the wrong way round",
			worstHazy, worstClean)
	}
}

// Longitude wraps rather than running off the end of the grid.
//
// A site just west of the prime meridian is at 359-and-a-bit degrees in the
// file's own convention, which is the last cell, not a negative index. Sites
// either side of the meridian should therefore read similarly rather than one
// of them failing or landing on the far side of the world.
func TestAODWrapsAtTheMeridian(t *testing.T) {
	testutil.RequireReachable(t, "eodata.dataspace.copernicus.eu:443")

	remote.EnableDownloads(50<<20, remote.CopernicusEODATA)

	west := aodAt(t, -0.5, 51.5) // just west of Greenwich
	east := aodAt(t, 0.5, 51.5)  // just east

	t.Logf("0.5W 51.5N: %.4f   0.5E 51.5N: %.4f", west, east)

	// One degree apart on a 0.4-degree grid: neighbouring cells, so the same
	// air mass. A wrap failure sends one of them to the antimeridian.
	if diff := west - east; diff > 0.2 || diff < -0.2 {
		t.Errorf("cells 1 degree apart read %.4f and %.4f; that is not one air mass, so "+
			"longitude is not wrapping", west, east)
	}
}

// The key names the freshest run covering an instant.
//
// Offline: this is string arithmetic over the archive's own layout, and it
// decides which file is fetched, so it is worth pinning without a network.
func TestAODKeyPicksTheCoveringCycle(t *testing.T) {
	t.Parallel()

	cases := []struct {
		when gotime.Time
		want string
	}{{
		// Exactly on the morning cycle: step zero of that cycle.
		when: gotime.Date(2023, 1, 1, 0, 0, 0, 0, gotime.UTC),
		want: "CAMS/GLOBAL/2023/01/01/z_cams_c_ecmf_20230101000000_prod_fc_sfc_000_aod550/" +
			"z_cams_c_ecmf_20230101000000_prod_fc_sfc_000_aod550.nc",
	}, {
		// Three hours later: same cycle, step three.
		when: gotime.Date(2023, 1, 1, 3, 0, 0, 0, gotime.UTC),
		want: "CAMS/GLOBAL/2023/01/01/z_cams_c_ecmf_20230101000000_prod_fc_sfc_003_aod550/" +
			"z_cams_c_ecmf_20230101000000_prod_fc_sfc_003_aod550.nc",
	}, {
		// Past midday: the afternoon cycle takes over, not the morning one
		// extended.
		when: gotime.Date(2023, 1, 1, 13, 30, 0, 0, gotime.UTC),
		want: "CAMS/GLOBAL/2023/01/01/z_cams_c_ecmf_20230101120000_prod_fc_sfc_001_aod550/" +
			"z_cams_c_ecmf_20230101120000_prod_fc_sfc_001_aod550.nc",
	}, {
		// A non-UTC instant resolves by its UTC time, not its wall clock.
		when: gotime.Date(2023, 1, 1, 20, 0, 0, 0, gotime.FixedZone("UTC+7", 7*3600)),
		want: "CAMS/GLOBAL/2023/01/01/z_cams_c_ecmf_20230101120000_prod_fc_sfc_001_aod550/" +
			"z_cams_c_ecmf_20230101120000_prod_fc_sfc_001_aod550.nc",
	}}

	for _, c := range cases {
		if got := cams.AODKey(c.when); got != c.want {
			t.Errorf("%s\n got %s\nwant %s", c.when.Format(gotime.RFC3339), got, c.want)
		}
	}
}
