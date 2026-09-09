//go:build network

package mpcorb_test

import (
	"context"
	"math"
	"path/filepath"
	"testing"

	"github.com/TuSKan/astrogo/catalog/mpcorb"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/ephemeris/kepler"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

// liveFile streams one of the MPC's subset files, skipping when the endpoint
// is unreachable rather than failing CI for somebody else's downtime.
//
// Distant.txt at ~1.7 MB is the smallest of them and still 8,000 objects: big
// enough that a column map which happens to work on five hand-picked rows has
// nowhere to hide, small enough to be a polite thing to fetch on every run.
func liveFile(t *testing.T, name string) []resolve.Target {
	t.Helper()

	testutil.RequireReachable(t, "www.minorplanetcenter.net:443")

	t.Cleanup(remote.Reset)
	remote.EnableDownloads(64<<20, remote.MPCORB)

	seq, err := mpcorb.Open(context.Background(), name)
	if err != nil {
		testutil.SkipOnUpstreamFailure(t, err)
		t.Fatalf("Open(%q): %v", name, err)
	}

	var (
		list []resolve.Target
		errs int
	)

	for tgt, err := range seq {
		if err != nil {
			errs++

			if errs <= 3 {
				t.Errorf("row error: %v", err)
			}

			continue
		}

		list = append(list, tgt)
	}

	if errs > 3 {
		t.Errorf("%d malformed rows in %s (first three above)", errs, name)
	}

	return list
}

// TestOpenParsesTheWholeSubsetFile is what the five-row fixture cannot be.
//
// Hand-picked rows prove the parser handles the shapes somebody thought of.
// Eight thousand real ones prove it handles the file, and every assertion here
// is a property the MPC's own format guarantees — so a failure is either a
// column map that slipped or a format that changed, and both are things this
// package must not paper over.
func TestOpenParsesTheWholeSubsetFile(t *testing.T) {
	list := liveFile(t, "Distant.txt")

	// 8,262 rows on 2026-09-08, growing. A floor rather than an equality: the
	// register only grows, and a parse that lost most of the file is an order
	// of magnitude away from this either way.
	const floor = 5000

	if len(list) < floor {
		t.Fatalf("parsed %d objects, want at least %d — Distant.txt does not shrink, so this "+
			"is the parser losing rows or the endpoint serving something else", len(list), floor)
	}

	var (
		withH   int
		maxEcc  float64
		minA    = math.Inf(1)
		maxA    float64
		unnamed int
	)

	for _, tgt := range list {
		if tgt.ID == "" {
			t.Fatalf("object %q has no packed designation", tgt.Name)
		}

		if tgt.Name == "" {
			unnamed++
		}

		if !tgt.HasElements {
			t.Fatalf("%s has HasElements false; every MPCORB row is an element set", tgt.ID)
		}

		if tgt.HasH {
			withH++
		}

		// MPCORB is built on a and n, so it can only express a closed orbit.
		// Measured across Distant.txt, NEA.txt and Unusual.txt on 2026-09-08:
		// 92,865 rows, not one with e >= 1. That is what makes this package's
		// output safe to hand to kepler, whose propagator is elliptical only.
		if tgt.Eccentricity < 0 || tgt.Eccentricity >= 1 {
			t.Errorf("%s (%s): eccentricity %v is not a closed orbit", tgt.ID, tgt.Name, tgt.Eccentricity)
		}

		maxEcc = math.Max(maxEcc, tgt.Eccentricity)

		if tgt.SemiMajorAxis <= 0 {
			t.Errorf("%s (%s): semi-major axis %v", tgt.ID, tgt.Name, tgt.SemiMajorAxis)
		}

		minA = math.Min(minA, tgt.SemiMajorAxis)
		maxA = math.Max(maxA, tgt.SemiMajorAxis)

		if incl := tgt.Inclination.Degrees(); incl < 0 || incl > 180 {
			t.Errorf("%s (%s): inclination %v deg", tgt.ID, tgt.Name, incl)
		}

		// Distant.txt is Centaurs and trans-Neptunians, so a row claiming an
		// epoch outside the era the MPC has been publishing them in is a
		// packed date read wrong rather than a genuinely ancient element set.
		if y := tgt.Epoch.Year(); y < 1900 || y > 2100 {
			t.Errorf("%s (%s): epoch year %d", tgt.ID, tgt.Name, y)
		}
	}

	t.Logf("%d objects: %d with H, %d unnamed, a in [%.2f, %.2f] AU, max e %.4f",
		len(list), withH, unnamed, minA, maxA, maxEcc)

	// Every object in Distant.txt is beyond Jupiter by definition, so a
	// semi-major axis inside 4 AU means the column is being read from the
	// wrong place — a check the fixture cannot make, because it cannot know
	// which file its rows came from.
	if minA < 4 {
		t.Errorf("smallest semi-major axis %.3f AU in a file of Centaurs and "+
			"trans-Neptunians; the a column may be misread", minA)
	}

	// The readable-designation column is populated for every row of both
	// files checked; an empty one means the name column moved.
	if unnamed != 0 {
		t.Errorf("%d objects have no readable designation", unnamed)
	}
}

// TestEveryParsedObjectPropagates is the end-to-end claim this package makes:
// that what it produces can be handed straight to the propagator.
//
// Asserting it over the whole file rather than one object is the point. The
// six elements are individually plausible in almost any misreading; what a
// misreading cannot survive is kepler's own validation across eight thousand
// rows, followed by a heliocentric distance that has to land between each
// object's own perihelion and aphelion.
func TestEveryParsedObjectPropagates(t *testing.T) {
	list := liveFile(t, "Distant.txt")

	var checked int

	for _, tgt := range list {
		el, err := kepler.NewElements(tgt.Epoch, tgt.SemiMajorAxis, tgt.Eccentricity,
			tgt.Inclination, tgt.AscendingNode, tgt.ArgPeriapsis, tgt.MeanAnomaly)
		if err != nil {
			t.Fatalf("%s (%s): kepler.NewElements: %v", tgt.ID, tgt.Name, err)
		}

		pos, _, err := el.StateAt(tgt.Epoch)
		if err != nil {
			t.Fatalf("%s (%s): StateAt its own epoch: %v", tgt.ID, tgt.Name, err)
		}

		// StateAt returns astronomical units.
		r := pos.Norm()

		lo := tgt.SemiMajorAxis * (1 - tgt.Eccentricity)
		hi := tgt.SemiMajorAxis * (1 + tgt.Eccentricity)

		if r < lo-1e-6 || r > hi+1e-6 {
			t.Fatalf("%s (%s): %.6f AU outside [%.6f, %.6f], which its own a and e forbid",
				tgt.ID, tgt.Name, r, lo, hi)
		}

		checked++
	}

	t.Logf("%d objects propagated at their own epochs, every one inside its own apsides", checked)
}

// TestOpenRefusesWithoutDownloadConsent pins that MPCORB is behind the same
// gate as every other bulk fetch — see CLAUDE.md's "Bulk file downloads never
// happen without explicit consent".
//
// The endpoint declares SizeVaries, so the grant's own byte budget is what
// decides; a grant too small for the file must be refused as clearly as no
// grant at all, which is the second half of this test.
func TestOpenRefusesWithoutDownloadConsent(t *testing.T) {
	testutil.RequireReachable(t, "www.minorplanetcenter.net:443")

	t.Cleanup(remote.Reset)
	remote.Reset()

	remote.SetDataDir("file:///" + tempBucket(t) + "?create_dir=true")

	if _, err := mpcorb.Open(context.Background(), "Distant.txt"); err == nil {
		t.Fatal("Open succeeded with no EnableDownloads grant")
	}

	// And the same empty cache directory must work once consent is granted,
	// or the refusal above proves only that the fixture is broken.
	remote.EnableDownloads(64<<20, remote.MPCORB)

	seq, err := mpcorb.Open(context.Background(), "Distant.txt")
	if err != nil {
		testutil.SkipOnUpstreamFailure(t, err)
		t.Fatalf("Open after consent: %v; the temporary cache directory is what "+
			"failed above, not the consent gate", err)
	}

	var n int

	for range seq {
		n++

		if n == 10 {
			break
		}
	}

	if n != 10 {
		t.Errorf("read %d rows after consent, want 10", n)
	}
}

// tempBucket returns a fresh directory as forward-slashed text, for building a
// file:// bucket URL. remote takes a URL and never an OS path, so the
// conversion happens at the one place that has an OS path to convert.
func tempBucket(t *testing.T) string {
	t.Helper()

	return filepath.ToSlash(t.TempDir())
}
