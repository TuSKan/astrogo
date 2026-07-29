package sbdb

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

func TestSBDBResolver(t *testing.T) {
	jsonData := `{
		"object": {
			"spkid": "20002062",
			"fullname": "2062 Aten (1976 AA)",
			"des": "2062",
			"kind": "a"
		}
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if _, err := fmt.Fprint(w, jsonData); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	t.Cleanup(remote.Reset)

	if err := remote.SetURL(remote.JPLSBDB, server.URL); err != nil {
		t.Fatal(err)
	}

	prov := New()

	tar, ok := prov.Resolve(context.Background(), "aten")
	if !ok {
		t.Fatalf("Failed to resolve Aten")
	}

	testutil.AssertEqual(t, "Resolve ID name", tar.Name, "2062 Aten (1976 AA)")
	testutil.AssertEqual(t, "Resolve SPKID", tar.SPKID, "20002062")

	// Regression: Kind must be the canonical resolve.KindAsteroid constant,
	// not an ad hoc resolve.Kind("Asteroid") string built outside the enum.
	if tar.Kind != resolve.KindAsteroid {
		t.Errorf("Kind = %q, want %q", tar.Kind, resolve.KindAsteroid)
	}

	// Test cache bypassing HTTP mock and testing async SeqIterator
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req := resolve.ObjectRequest{Query: "aten"}
	iter := prov.ResolveObject(ctx, req)

	var targets []resolve.Target

	iter(func(tar resolve.Target, err error) bool {
		if err == nil {
			targets = append(targets, tar)
		}

		return true
	})

	if len(targets) != 1 {
		t.Fatalf("Expected 1 targets from stream, got %d", len(targets))
	}
}

func TestSBDBResolver_CometKind(t *testing.T) {
	jsonData := `{
		"object": {
			"spkid": "1000036",
			"fullname": "1P/Halley",
			"des": "1P",
			"kind": "c"
		}
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if _, err := fmt.Fprint(w, jsonData); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	t.Cleanup(remote.Reset)

	if err := remote.SetURL(remote.JPLSBDB, server.URL); err != nil {
		t.Fatal(err)
	}

	prov := New()

	tar, ok := prov.Resolve(context.Background(), "halley")
	if !ok {
		t.Fatalf("Failed to resolve Halley")
	}

	if tar.Kind != resolve.KindComet {
		t.Errorf("Kind = %q, want %q", tar.Kind, resolve.KindComet)
	}
}

func TestClassifyKind(t *testing.T) {
	tests := []struct {
		name         string
		spkID        string
		orbitClass   string
		eccentricity float64
		isComet      bool
		want         resolve.Kind
	}{
		{"Ceres", "20000001", "MBA", 0.08, false, resolve.KindDwarfPlanet},
		{"Pluto", "20134340", "TNO", 0.25, false, resolve.KindDwarfPlanet},
		{"Eris", "20136199", "TNO", 0.44, false, resolve.KindDwarfPlanet},
		{"Haumea", "20136108", "TNO", 0.2, false, resolve.KindDwarfPlanet},
		{"Makemake", "20136472", "TNO", 0.16, false, resolve.KindDwarfPlanet},
		{"Vesta", "20000004", "MBA", 0.09, false, resolve.KindAsteroid},
		{"ordinary comet", "1000036", "HTC", 0.97, true, resolve.KindComet},
		{"a comet SPK-ID that happens to match a dwarf-planet ID", "20000001", "JFC", 0.5, true, resolve.KindComet},
		// Real orbit_class codes and eccentricities confirmed live against
		// JPL SBDB: 1I/'Oumuamua reports "HYA" (Hyperbolic Asteroid),
		// e=1.2; 2I/Borisov reports "HYP" (Hyperbolic Comet), e=3.36 —
		// both take priority over the asteroid/comet/dwarf-planet
		// classification.
		{"'Oumuamua (hyperbolic asteroid)", "50788063", "HYA", 1.2, false, resolve.KindInterstellar},
		{"Borisov (hyperbolic comet)", "1003639", "HYP", 3.36, true, resolve.KindInterstellar},
		{"a hyperbolic orbit that also happens to be a dwarf-planet SPK-ID", "20000001", "HYA", 1.2, false, resolve.KindInterstellar},
		// Regression case: a real false positive confirmed live against
		// SBDB — an ordinary long-period (Oort-cloud) comet whose current
		// best-fit osculating orbit sits fractionally above e=1 (a known
		// fitting/perturbation artifact, e.g. real C/1937 C1 Whipple at
		// e=1.0002) is NOT interstellar and must not be classified as one,
		// even though JPL itself labels its orbit_class "HYP".
		{"marginally-hyperbolic long-period comet (not interstellar)", "1001030", "HYP", 1.0002, true, resolve.KindComet},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyKind(tt.spkID, tt.orbitClass, tt.eccentricity, tt.isComet); got != tt.want {
				t.Errorf("classifyKind(%q, %q, %v, %v) = %q, want %q", tt.spkID, tt.orbitClass, tt.eccentricity, tt.isComet, got, tt.want)
			}
		})
	}
}

func TestSearchBrightMock(t *testing.T) {
	asteroidJSON := `{
		"signature": {"version": "1.0"},
		"fields": ["full_name", "spkid", "H", "G", "class", "e"],
		"data": [
			["1 Ceres", "20000001", 3.34, 0.12, "MBA", 0.08],
			["4 Vesta", "20000004", 3.25, 0.32, "MBA", 0.09],
			["'Oumuamua (A/2017 U1)", "50788063", 22.08, 0.15, "HYA", 1.2]
		],
		"count": 3
	}`
	cometJSON := `{
		"signature": {"version": "1.0"},
		"fields": ["full_name", "spkid", "M1", "K1", "class", "e"],
		"data": [
			["1P/Halley", "1000036", 4.5, 8.0, "HTC", 0.97]
		],
		"count": 1
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		body := asteroidJSON
		if r.URL.Query().Get("sb-kind") == "c" {
			body = cometJSON
		}

		if _, err := fmt.Fprint(w, body); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	t.Cleanup(remote.Reset)

	if err := remote.SetURL(remote.JPLSBDBQuery, server.URL); err != nil {
		t.Fatal(err)
	}

	prov := New()

	var got []resolve.Target

	iter := prov.SearchBright(context.Background(), resolve.BrightRequest{MaxVMag: -2})
	iter(func(tgt resolve.Target, err error) bool {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got = append(got, tgt)

		return true
	})

	if len(got) != 4 {
		t.Fatalf("expected 4 targets (3 asteroids + 1 comet), got %d: %+v", len(got), got)
	}

	// 1 Ceres (SPK-ID 20000001) is one of the five IAU-recognized dwarf
	// planets, reported as resolve.KindDwarfPlanet rather than the generic
	// resolve.KindAsteroid every other numbered minor planet gets.
	ceres := got[0]
	if ceres.Name != "1 Ceres" || ceres.SPKID != "20000001" || ceres.Kind != resolve.KindDwarfPlanet {
		t.Errorf("unexpected dwarf planet target: %+v", ceres)
	}

	if !ceres.HasH || ceres.H != 3.34 || ceres.G != 0.12 {
		t.Errorf("expected H=3.34 G=0.12 HasH=true, got H=%v G=%v HasH=%v", ceres.H, ceres.G, ceres.HasH)
	}

	if ceres.HasM1 {
		t.Errorf("expected a dwarf planet to not have M1 set, got HasM1=true")
	}

	vesta := got[1]
	if vesta.Name != "4 Vesta" || vesta.SPKID != "20000004" || vesta.Kind != resolve.KindAsteroid {
		t.Errorf("unexpected asteroid target: %+v", vesta)
	}

	// 'Oumuamua reports orbit_class "HYA" (Hyperbolic Asteroid) — takes
	// priority over the plain-asteroid classification its sb-kind alone
	// would otherwise give it.
	oumuamua := got[2]
	if oumuamua.SPKID != "50788063" || oumuamua.Kind != resolve.KindInterstellar {
		t.Errorf("unexpected interstellar target: %+v", oumuamua)
	}

	halley := got[3]
	if halley.Name != "1P/Halley" || halley.Kind != resolve.KindComet {
		t.Errorf("unexpected comet target: %+v", halley)
	}

	if !halley.HasM1 || halley.M1 != 4.5 || halley.K1 != 8.0 {
		t.Errorf("expected M1=4.5 K1=8.0 HasM1=true, got M1=%v K1=%v HasM1=%v", halley.M1, halley.K1, halley.HasM1)
	}

	if halley.HasH {
		t.Errorf("expected a comet to not have H set, got HasH=true")
	}
}

func TestProviderInterface(t *testing.T) {
	p := New()
	if p.Name() != "sbdb" {
		t.Errorf("expected sbdb, got %s", p.Name())
	}

	caps := p.Capabilities()
	if len(caps) != 2 || caps[0] != resolve.CapObjectResolution || caps[1] != resolve.CapMagnitudeBrowse {
		t.Errorf("expected CapObjectResolution and CapMagnitudeBrowse, got %v", caps)
	}

	_, _ = p.Resolve(context.Background(), "non_existent_body")
	_ = p.Search(context.Background(), "non_existent_body")
}
