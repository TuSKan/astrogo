//go:build network

package plan

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

// requireMPCList fetches the real observatory-code list once per run, skipping
// when the MPC is unreachable rather than failing CI for somebody else's
// downtime.
func requireMPCList(t *testing.T) []MPCObservatory {
	t.Helper()

	testutil.RequireReachable(t, "www.minorplanetcenter.net:443")

	t.Cleanup(remote.Capture(remote.MPCObsCodes).Restore)
	remote.EnableDownloads(0, remote.MPCObsCodes)

	list, err := MPCObservatories(context.Background())
	if err != nil {
		testutil.SkipOnUpstreamFailure(t, err)
		t.Fatalf("MPCObservatories: %v", err)
	}

	return list
}

// TestMPCObservatoriesParsesTheWholeRegister is the test the offline fixture
// cannot be: ten hand-picked rows prove the parser handles the shapes somebody
// thought of, and this proves it handles the ~2,700 that actually exist.
//
// It asserts a floor rather than an exact count. The register grows by a few
// codes a month, so an equality check would fail for the one reason that is
// not a defect, and the thing worth catching — a parse that quietly loses most
// of the file — is an order of magnitude away from the boundary either way.
func TestMPCObservatoriesParsesTheWholeRegister(t *testing.T) {
	list := requireMPCList(t)

	// 2,722 rows on 2026-09-08. A tenth of that is not a shrinking register,
	// it is a parser reading a different document.
	const floor = 2000

	if len(list) < floor {
		t.Fatalf("parsed %d observatories, want at least %d — the register does not shrink, "+
			"so this is the parser losing rows or the endpoint serving something else", len(list), floor)
	}

	var (
		positioned int
		blank      int
	)

	for _, obs := range list {
		if obs.Code == "" {
			t.Errorf("observatory %q has an empty code", obs.Name)
		}

		if obs.Name == "" {
			t.Errorf("observatory %q has an empty name", obs.Code)
		}

		if obs.Location == nil {
			blank++

			continue
		}

		positioned++

		// Every recovered position must be physically possible. A field read
		// one column off produces numbers that still parse, and this is what
		// separates that from a correct read.
		if lat := obs.Location.Lat().Degrees(); math.Abs(lat) > 90 {
			t.Errorf("%s %s: latitude %.4f out of range", obs.Code, obs.Name, lat)
		}

		// Height is bounded by the row's own published resolution rather than
		// by a flat number, because a flat number cannot be both true and
		// useful here. 507 Nyenheim recovers to -6,655 m and is not an error:
		// its constants carry three decimals, so the row is worth +/-3.2 km
		// and -6.6 km is two units in the last place. Holding a 3-decimal row
		// to the same bound as a 6-decimal one either fails on correct data
		// or waves through a genuine column misread.
		//
		// The three geocentre rows (244, 248, 500) publish constants that are
		// exactly zero and land 6,378 km down. That is the origin, faithfully
		// reported, and it is excluded by name rather than by widening the
		// bound until it happens to fit.
		switch obs.Code {
		case "244", "248", "500":
			continue
		}

		// The band is where places on Earth are, widened by this row's own
		// published resolution. Measured over the 2,689 positioned non-geocentre
		// rows on 2026-09-08, grouped by how many decimals their constants carry:
		//
		//	decimals      n   recovered height
		//	       6   1317   -33 m .. +5,351 m
		//	       5   1244   -207 m .. +4,212 m
		//	       4     90   -361 m .. +3,852 m
		//	       3     38   -9,254 m .. +9,614 m
		//
		// 95% of the register lands inside a kilometre of sea level and six of
		// the summit of Everest; the outliers are all in the two coarse
		// buckets, and every one of them is inside its own quantisation. That
		// is the check: not "is this height plausible" — which cannot be true
		// for a 3-decimal row — but "is it plausible given what this row
		// claims to know". A column read one field over produces hundreds of
		// kilometres and fails at any precision.
		const (
			deepest = -1000.0
			highest = 6000.0
			slack   = 4.0
		)

		lo := deepest - slack*obs.ResolutionM
		hi := highest + slack*obs.ResolutionM

		if h := obs.Location.Height(); h < lo || h > hi {
			t.Errorf("%s %s: height %.0f m outside [%.0f, %.0f] — %.0f m of sea-level "+
				"band plus %gx this row's own +/-%.0f m resolution",
				obs.Code, obs.Name, h, lo, hi, highest-deepest, slack, obs.ResolutionM)
		}
	}

	t.Logf("%d observatories: %d positioned, %d without a ground position", len(list), positioned, blank)

	// The positionless rows are the space telescopes, the geocentre and the
	// roving-observer placeholders — 30 of them on 2026-09-08. A parser that
	// started reading blanks as zeros would drive this to nought, and one
	// reading real constants as blanks would drive it up.
	if blank == 0 || blank > 100 {
		t.Errorf("%d rows without a ground position; expected a few dozen "+
			"(space telescopes, the geocentre, roving observers)", blank)
	}
}

// TestNewMPCSiteResolvesRealCodes checks the whole path — fetch, parse, recover,
// build — against codes whose positions this package independently holds.
func TestNewMPCSiteResolvesRealCodes(t *testing.T) {
	requireMPCList(t)

	for _, tc := range []struct {
		code  string
		known string
		tolD  float64
	}{
		{"000", "greenwich", 0.01},
		{"309", "paranal", 0.01},
		{"568", "mauna_kea", 0.01},
		{"807", "cerro_tololo", 0.01},
	} {
		t.Run(tc.code, func(t *testing.T) {
			site, err := NewMPCSite(context.Background(), tc.code)
			if err != nil {
				t.Fatalf("NewMPCSite(%q): %v", tc.code, err)
			}

			want, ok := KnownSites[tc.known]
			if !ok {
				t.Skipf("KnownSites has no %q to compare against", tc.known)
			}

			testutil.AssertNear(t, "latitude",
				site.Latitude().Degrees(), want.Location().Lat().Degrees(), tc.tolD)

			if site.MPCCode() != tc.code {
				t.Errorf("MPCCode = %q, want %q", site.MPCCode(), tc.code)
			}
		})
	}
}

// TestNewMPCSiteRejectsCodesWithNoGroundPosition runs the error-vs-absence
// distinction against the live list rather than the fixture, so it also
// confirms these rows survive the real file's formatting.
func TestNewMPCSiteRejectsCodesWithNoGroundPosition(t *testing.T) {
	requireMPCList(t)

	for _, code := range []string{"250", "258", "274"} { // Hubble, Gaia, JWST
		_, err := NewMPCSite(context.Background(), code)
		if !errors.Is(err, ErrSiteNotOnEarth) {
			t.Errorf("NewMPCSite(%q) err = %v, want ErrSiteNotOnEarth", code, err)
		}
	}

	if _, err := NewMPCSite(context.Background(), "!!!"); !errors.Is(err, ErrUnknownSite) {
		t.Errorf("NewMPCSite(%q) err = %v, want ErrUnknownSite", "!!!", err)
	}
}

// TestMPCObservatoriesNeedsDownloadConsent pins that the list is behind the
// same gate as every other bulk fetch — see CLAUDE.md's "Bulk file downloads
// never happen without explicit consent".
//
// The cache directory is pointed at an empty temporary bucket, which is the
// whole point rather than tidiness: consent gates the *download*, not reading
// something already cached. Run against a warm cache this test passes with the
// gate removed, so it would be asserting nothing — which is exactly what it did
// on the first attempt, and why the temp bucket is here.
func TestMPCObservatoriesNeedsDownloadConsent(t *testing.T) {
	testutil.RequireReachable(t, "www.minorplanetcenter.net:443")

	// Scoped to this one endpoint rather than remote.Reset, which restores
	// the process default — no consent — and so revokes what
	// integration_main_test.go's TestMain granted for the whole binary.
	// Reset also leaves the data directory alone, so the empty bucket below
	// would stay pointed there afterwards: a later test would find neither
	// consent nor cache. That combination failed three unrelated eclipse and
	// moon-phase tests (#239), which is the second time the same three have
	// been broken this way — see visible_tonight_internal_test.go's note.
	t.Cleanup(remote.Capture(remote.MPCObsCodes).Restore)
	remote.DisableDownloads(remote.MPCObsCodes)

	remote.SetDataDir("file:///" + filepath.ToSlash(t.TempDir()) + "?create_dir=true")

	// The parsed list is process-wide, so a previous test in this package may
	// already hold it; clearing it is what makes the fetch happen at all.
	mpcCache.mu.Lock()
	mpcCache.list = nil
	mpcCache.mu.Unlock()

	t.Cleanup(func() {
		mpcCache.mu.Lock()
		mpcCache.list = nil
		mpcCache.mu.Unlock()
	})

	_, err := MPCObservatories(context.Background())
	if !errors.Is(err, remote.ErrDownloadDenied) {
		t.Fatalf("err = %v, want remote.ErrDownloadDenied without EnableDownloads", err)
	}

	// And the same empty bucket must work once consent is granted, or the
	// assertion above would hold for a test that had simply broken the cache
	// directory — a denial produced by the fixture rather than by the gate.
	remote.EnableDownloads(0, remote.MPCObsCodes)

	list, err := MPCObservatories(context.Background())
	if err != nil {
		testutil.SkipOnUpstreamFailure(t, err)
		t.Fatalf("MPCObservatories after consent: %v; the temporary cache directory "+
			"is what failed above, not the consent gate", err)
	}

	if len(list) == 0 {
		t.Error("consent granted and the list came back empty")
	}
}

// TestMPCObservatoriesCoverTheLatitudesKnownSitesDoesNot is the half of #125
// that is about the test corpus rather than the count.
//
// The ten hand-curated sites are all mid-latitude or tropical, so the
// circumpolar and never-rises paths had only synthetic latitudes to run
// against (circumpolarTestSite builds them). The register supplies real ones:
// on 2026-09-08, 27 sites above +60N reaching EISCAT Tromso at +69.6, and two
// below -60S — Concordia at -75.1 and Kunlun Station at -80.4 — both inside
// the Antarctic Circle.
//
// Asserted as floors, since the register only grows, and asserted at all
// because a site table with no polar entries makes a whole class of
// observability answer untestable against anything real.
//
// The southern floor is two because that is what the register holds. The
// first draft of this test said five, counted by a script that had not
// excluded the three geocentre rows sitting at -90 by construction. The test
// found it, which is the argument for asserting the number at all.
func TestMPCObservatoriesCoverTheLatitudesKnownSitesDoesNot(t *testing.T) {
	list := requireMPCList(t)

	var (
		arctic    int
		antarctic int
		equator   int
		maxNorth  MPCObservatory
		maxSouth  MPCObservatory
	)

	for _, obs := range list {
		if obs.Location == nil {
			continue
		}

		// The three geocentre rows sit at the pole by construction and would
		// otherwise be counted as the most extreme site in both hemispheres.
		switch obs.Code {
		case "244", "248", "500":
			continue
		}

		lat := obs.Location.Lat().Degrees()

		switch {
		case lat > 60:
			arctic++
		case lat < -60:
			antarctic++
		}

		if math.Abs(lat) < 5 {
			equator++
		}

		if maxNorth.Location == nil || lat > maxNorth.Location.Lat().Degrees() {
			maxNorth = obs
		}

		if maxSouth.Location == nil || lat < maxSouth.Location.Lat().Degrees() {
			maxSouth = obs
		}
	}

	t.Logf("northernmost %s %s at %+.3f; southernmost %s %s at %+.3f",
		maxNorth.Code, maxNorth.Name, maxNorth.Location.Lat().Degrees(),
		maxSouth.Code, maxSouth.Name, maxSouth.Location.Lat().Degrees())
	t.Logf("%d sites above +60, %d below -60, %d within 5 of the equator", arctic, antarctic, equator)

	for _, tc := range []struct {
		what string
		got  int
		want int
	}{
		{"sites above +60 latitude", arctic, 20},
		{"sites below -60 latitude", antarctic, 2},
		{"sites within 5 degrees of the equator", equator, 5},
	} {
		if tc.got < tc.want {
			t.Errorf("%s: %d, want at least %d", tc.what, tc.got, tc.want)
		}
	}

	// At least one real site inside a polar circle, which is what makes a
	// circumpolar target circumpolar rather than merely high.
	if maxNorth.Location.Lat().Degrees() < 66.5 && maxSouth.Location.Lat().Degrees() > -66.5 {
		t.Errorf("no site inside either polar circle: northernmost %+.3f, southernmost %+.3f",
			maxNorth.Location.Lat().Degrees(), maxSouth.Location.Lat().Degrees())
	}
}
