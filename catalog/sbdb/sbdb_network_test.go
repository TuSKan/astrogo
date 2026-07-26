//go:build network
// +build network

package sbdb

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

// requireSBDB skips the test when the JPL SBDB API is unreachable — per
// this project's network test policy, a reachability failure must never
// fail CI outright.
func requireSBDB(t *testing.T) {
	t.Helper()

	conn, err := net.DialTimeout("tcp", "ssd-api.jpl.nasa.gov:443", 5*time.Second)
	if err != nil {
		t.Skipf("SBDB unreachable, skipping live test: %v", err)
	}

	_ = conn.Close()
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

	sawAsteroid, sawComet := false, false

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
}
