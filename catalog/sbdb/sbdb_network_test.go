//go:build network

package sbdb

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/internal/testutil"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

// requireSBDB skips the test when the JPL SBDB API is unreachable — per
// this project's network test policy, a reachability failure must never
// fail CI outright.
func requireSBDB(t *testing.T) {
	t.Helper()

	testutil.RequireReachable(t, "ssd-api.jpl.nasa.gov:443")
}

func TestSBDBNetworkResolve(t *testing.T) {
	requireSBDB(t)

	prov := New()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := resolve.ObjectRequest{Query: "Aten", Limit: 1}
	iter := prov.ResolveObject(ctx, req)

	var targets []resolve.Target

	iter(func(tar resolve.Target, err error) bool {
		if err != nil {
			t.Fatalf("Live network failed: %v", err)
		}

		targets = append(targets, tar)

		return true
	})

	if len(targets) == 0 {
		t.Fatalf("Expected remote result for Halley")
	}

	tgt := targets[0]
	if tgt.ID == "" {
		t.Errorf("Expected ID populated from live server")
	}
}

// TestSBDBNetworkResolveOrbitalElements confirms the live SBDB identify
// API returns a complete, plausible orbital-element set for a
// well-known numbered asteroid — 1 Ceres, whose real elements are
// stable and well-documented (a ~2.77 au, e ~0.08, i ~10.6 deg).
// Tolerances are loose since SBDB periodically re-fits and republishes
// its best orbit solution.
func TestSBDBNetworkResolveOrbitalElements(t *testing.T) {
	requireSBDB(t)

	prov := New()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tar, ok := prov.Resolve(ctx, "1")
	if !ok {
		t.Fatal("failed to resolve 1 Ceres")
	}

	if !tar.HasElements {
		t.Fatal("HasElements = false, want true for a well-numbered asteroid")
	}

	if tar.SemiMajorAxis < 2.5 || tar.SemiMajorAxis > 3.0 {
		t.Errorf("SemiMajorAxis = %v AU, outside plausible band for 1 Ceres (~2.77 AU)", tar.SemiMajorAxis)
	}

	if tar.Eccentricity < 0 || tar.Eccentricity > 0.2 {
		t.Errorf("Eccentricity = %v, outside plausible band for 1 Ceres (~0.08)", tar.Eccentricity)
	}

	if incl := tar.Inclination.Degrees(); incl < 9 || incl > 12 {
		t.Errorf("Inclination = %v deg, outside plausible band for 1 Ceres (~10.6 deg)", incl)
	}

	if tar.Epoch.IsZero() {
		t.Error("Epoch is zero, want the elements' real epoch of osculation")
	}
}

// TestSBDBNetworkSearchBright confirms the bulk asteroid+comet query API
// (a distinct endpoint from the identify API TestSBDBNetworkResolve
// exercises) is reachable and returns real, correctly-typed data.
func TestSBDBNetworkSearchBright(t *testing.T) {
	requireSBDB(t)

	prov := New()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var got []resolve.Target

	// MaxVMag: 1 (not -2) — brightnessMargin was deliberately tightened
	// from 7.0 to 4.0 (calibrated against real asteroid opposition
	// brightening; see the const's doc comment), and comets' M1 scale
	// runs fainter than asteroids' H for the well-known bright bodies —
	// confirmed live that -2+4=2 leaves zero comets with M1<2 today,
	// while 1+4=5 comfortably includes real ones (e.g. 31P/Schwassmann-
	// Wachmann 2) alongside real bright asteroids (H<5 includes Pluto).
	iter := prov.SearchBright(ctx, resolve.BrightRequest{MaxVMag: 1, Limit: 20})
	iter(func(tgt resolve.Target, err error) bool {
		if err != nil {
			t.Fatalf("live SearchBright failed: %v", err)
		}

		got = append(got, tgt)

		return true
	})

	if len(got) == 0 {
		t.Fatal("expected at least one real asteroid/comet from the live SBDB query API")
	}

	sawAsteroid, sawComet, sawElements := false, false, false

	for _, tgt := range got {
		if tgt.Name == "" || tgt.SPKID == "" {
			t.Errorf("expected populated Name/SPKID from live server, got %+v", tgt)
		}

		switch tgt.Kind {
		case resolve.KindAsteroid, resolve.KindDwarfPlanet:
			// At MaxVMag 1, Stage 1's H-sorted-ascending result set is
			// dominated by the brightest (lowest-H) known asteroids —
			// which is exactly the five IAU dwarf planets (Ceres, Pluto,
			// Eris, Haumea, Makemake), each reported as KindDwarfPlanet
			// rather than the generic KindAsteroid — so both are expected
			// here, not just KindAsteroid.
			sawAsteroid = true

			if !tgt.HasH {
				t.Errorf("expected a live asteroid/dwarf planet to have H set: %+v", tgt)
			}
		case resolve.KindComet:
			sawComet = true

			if !tgt.HasM1 {
				t.Errorf("expected a live comet to have M1 set: %+v", tgt)
			}
		default:
			t.Errorf("unexpected Kind from SearchBright: %+v", tgt)
		}

		// Phase 2 of the ephemeris-integration plan: queryBright's bulk
		// path must carry real orbital elements too, not just identity +
		// photometry — this is the live confirmation that sbdb_query.api
		// genuinely accepts a/i/om/w/ma/epoch as field names (verified
		// once by hand via curl during implementation; this test is what
		// keeps that confirmed going forward).
		//
		// No narrow positive band on SemiMajorAxis: a live run at this
		// MaxVMag genuinely includes distant TNOs/Sednoids (a up to
		// ~1000 AU) and hyperbolic comets, whose SBDB convention is a
		// *negative* semi-major axis — real diversity this decoder must
		// carry faithfully, not reject (rejecting e>=1 is
		// ephemeris/kepler.Validate's job downstream, not this test's).
		// Only assert the value is finite and nonzero.
		if tgt.HasElements {
			sawElements = true

			if tgt.SemiMajorAxis == 0 || math.IsNaN(tgt.SemiMajorAxis) || math.IsInf(tgt.SemiMajorAxis, 0) {
				t.Errorf("%s: SemiMajorAxis = %v, expected a finite nonzero value", tgt.Name, tgt.SemiMajorAxis)
			}

			if tgt.Epoch.IsZero() {
				t.Errorf("%s: HasElements=true but Epoch is zero", tgt.Name)
			}
		}
	}

	// At the generous prefilter this test uses (MaxVMag 1 + the
	// package's brightnessMargin), real known bodies like Pluto and
	// several periodic comets should appear on both sides — if either
	// never shows up, something's wrong with that sb-kind branch, not
	// just "no bright enough body exists right now".
	if !sawAsteroid {
		t.Error("expected at least one asteroid in the live result set")
	}

	if !sawComet {
		t.Error("expected at least one comet in the live result set")
	}

	if !sawElements {
		t.Error("expected at least one candidate with HasElements=true from the live bulk query — queryBright's element fields may not be parsing")
	}
}

// TestSBDBNetworkResolveInterstellar confirms both P2 fixes against the
// real API: 1I/'Oumuamua's live SBDB record reports its "kind" field as
// "au" (asteroid, unnumbered) — the isComet strict-"c"-equality bug this
// session found and fixed would have silently misparsed that regardless —
// and its orbit_class.code as "HYA" (Hyperbolic Asteroid), which
// classifyKind must map to resolve.KindInterstellar, not resolve.KindAsteroid.
func TestSBDBNetworkResolveInterstellar(t *testing.T) {
	requireSBDB(t)

	prov := New()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	iter := prov.ResolveObject(ctx, resolve.ObjectRequest{Query: "1I", Limit: 1})

	var targets []resolve.Target

	iter(func(tar resolve.Target, err error) bool {
		if err != nil {
			t.Fatalf("Live network failed: %v", err)
		}

		targets = append(targets, tar)

		return true
	})

	if len(targets) == 0 {
		t.Fatal("expected a live result for 1I/'Oumuamua")
	}

	if got := targets[0].Kind; got != resolve.KindInterstellar {
		t.Errorf("1I/'Oumuamua Kind = %v, want %v", got, resolve.KindInterstellar)
	}
}
