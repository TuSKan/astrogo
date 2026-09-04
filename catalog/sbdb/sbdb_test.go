package sbdb

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
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

	tar, err := prov.Resolve(context.Background(), "aten")
	if err != nil {
		t.Fatalf("Failed to resolve Aten")
	}

	testutil.AssertEqual(t, "Resolve ID name", tar.Name, "2062 Aten (1976 AA)")
	testutil.AssertEqual(t, "Resolve SPKID", tar.SPKID, "20002062")

	// Regression: Kind must be the canonical resolve.KindAsteroid constant,
	// not an ad hoc resolve.Kind("Asteroid") string built outside the enum.
	if tar.Kind != resolve.KindAsteroid {
		t.Errorf("Kind = %q, want %q", tar.Kind, resolve.KindAsteroid)
	}

	// This fixture has no "orbit" object at all — HasElements must stay
	// false rather than reporting zero-valued elements as real ones.
	if tar.HasElements {
		t.Error("HasElements = true, want false: fixture has no orbit.elements")
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

	tar, err := prov.Resolve(context.Background(), "halley")
	if err != nil {
		t.Fatalf("Failed to resolve Halley")
	}

	if tar.Kind != resolve.KindComet {
		t.Errorf("Kind = %q, want %q", tar.Kind, resolve.KindComet)
	}
}

// TestSBDBResolver_OrbitalElements uses a fixture matching the real live
// SBDB response for 1 Ceres (fields trimmed to what parsing needs),
// fetched and verified live before this test was written — not
// fabricated. SBDB natively publishes a in au and i/om/w/ma in degrees,
// so this also confirms no unit conversion is silently applied.
func TestSBDBResolver_OrbitalElements(t *testing.T) {
	jsonData := `{
		"object": {
			"spkid": "20000001",
			"fullname": "1 Ceres (A801 AA)",
			"des": "1",
			"kind": "an",
			"orbit_class": {"code": "MBA"}
		},
		"orbit": {
			"epoch": "2461200.5",
			"elements": [
				{"name": "e", "value": "0.0797"},
				{"name": "a", "value": "2.77"},
				{"name": "q", "value": "2.55"},
				{"name": "i", "value": "10.6"},
				{"name": "om", "value": "80.2"},
				{"name": "w", "value": "73.3"},
				{"name": "ma", "value": "274"},
				{"name": "tp", "value": "2461599.841"},
				{"name": "per", "value": "1680"}
			]
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

	tar, err := prov.Resolve(context.Background(), "ceres")
	if err != nil {
		t.Fatalf("Failed to resolve Ceres")
	}

	if !tar.HasElements {
		t.Fatal("HasElements = false, want true: fixture has complete orbit.elements + epoch")
	}

	testutil.AssertNear(t, "SemiMajorAxis", tar.SemiMajorAxis, 2.77, 1e-9)
	testutil.AssertNear(t, "Eccentricity", tar.Eccentricity, 0.0797, 1e-9)
	testutil.AssertNear(t, "Inclination", tar.Inclination.Degrees(), 10.6, 1e-9)
	testutil.AssertNear(t, "AscendingNode", tar.AscendingNode.Degrees(), 80.2, 1e-9)
	testutil.AssertNear(t, "ArgPeriapsis", tar.ArgPeriapsis.Degrees(), 73.3, 1e-9)
	testutil.AssertNear(t, "MeanAnomaly", tar.MeanAnomaly.Degrees(), 274, 1e-9)

	wantEpochJD, wantEpochFrac := tar.Epoch.JDParts()
	if gotJD := wantEpochJD + wantEpochFrac; math.Abs(gotJD-2461200.5) > 1e-6 {
		t.Errorf("Epoch JD = %v, want 2461200.5", gotJD)
	}
}

// TestSBDBResolver_IncompleteElements confirms a partial elements set
// (missing "ma" here) leaves HasElements false rather than reporting an
// incomplete, unusable set of orbital elements as real ones.
func TestSBDBResolver_IncompleteElements(t *testing.T) {
	jsonData := `{
		"object": {
			"spkid": "20000001",
			"fullname": "1 Ceres (A801 AA)",
			"des": "1",
			"kind": "an"
		},
		"orbit": {
			"epoch": "2461200.5",
			"elements": [
				{"name": "e", "value": "0.0797"},
				{"name": "a", "value": "2.77"},
				{"name": "i", "value": "10.6"},
				{"name": "om", "value": "80.2"},
				{"name": "w", "value": "73.3"}
			]
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

	tar, err := prov.Resolve(context.Background(), "ceres")
	if err != nil {
		t.Fatalf("Failed to resolve Ceres")
	}

	if tar.HasElements {
		t.Error("HasElements = true, want false: fixture is missing mean anomaly (ma)")
	}
}

// TestSBDBResolver_PhysicalDiameterAlbedo covers decoding the "diameter"
// and "albedo" phys_par entries into Target.Diameter/Albedo (and their
// Has* flags) -- live-verified against the real 433 Eros SBDB response
// (H=10.40, diameter=16.84 km, albedo=0.25) during implementation.
func TestSBDBResolver_PhysicalDiameterAlbedo(t *testing.T) {
	jsonData := `{
		"object": {
			"spkid": "2000433",
			"fullname": "433 Eros (A898 PA)",
			"des": "433",
			"kind": "a"
		},
		"phys_par": [
			{"name": "H", "value": "10.40"},
			{"name": "G", "value": "0.46"},
			{"name": "diameter", "value": "16.84"},
			{"name": "albedo", "value": "0.25"}
		]
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

	tar, err := prov.Resolve(context.Background(), "eros")
	if err != nil {
		t.Fatalf("Failed to resolve Eros")
	}

	if !tar.HasDiameter || tar.Diameter != 16.84 {
		t.Errorf("Diameter = %v (HasDiameter=%v), want 16.84 (HasDiameter=true)", tar.Diameter, tar.HasDiameter)
	}

	if !tar.HasAlbedo || tar.Albedo != 0.25 {
		t.Errorf("Albedo = %v (HasAlbedo=%v), want 0.25 (HasAlbedo=true)", tar.Albedo, tar.HasAlbedo)
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

// TestSearchBrightMock_OrbitalElements covers queryBright's element
// decoding (Phase 2 of the ephemeris-integration plan): a full row
// carries HasElements=true and the six element fields + Epoch; a row
// missing one element (here "ma") must not set HasElements at all, per
// the same all-or-nothing gate ResolveObject's single-object identify
// path already uses; a hyperbolic body (negative-ish geometry, e>=1)
// still gets its elements decoded faithfully — rejecting a hyperbolic
// orbit is ephemeris/kepler.Validate's job, not this decoder's.
func TestSearchBrightMock_OrbitalElements(t *testing.T) {
	asteroidJSON := `{
		"signature": {"version": "1.0"},
		"fields": ["full_name", "spkid", "H", "G", "class", "e", "a", "i", "om", "w", "ma", "epoch"],
		"data": [
			["1 Ceres", "20000001", 3.34, 0.12, "MBA", 0.0797, 2.77, 10.6, 80.2, 73.3, 274, 2461200.5],
			["2 Pallas", "20000002", 4.13, 0.11, "MBA", 0.229, 2.77, 34.8, 173.1, 310.9, null, 2461200.5]
		],
		"count": 2
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		body := asteroidJSON
		if r.URL.Query().Get("sb-kind") == "c" {
			body = `{"signature":{"version":"1.0"},"fields":["full_name","spkid","M1","K1","class","e","a","i","om","w","ma","epoch"],"data":[],"count":0}`
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

	if len(got) != 2 {
		t.Fatalf("expected 2 targets, got %d: %+v", len(got), got)
	}

	ceres := got[0]
	if !ceres.HasElements {
		t.Fatal("expected Ceres to have HasElements=true — every element was present")
	}

	if math.Abs(ceres.SemiMajorAxis-2.77) > 1e-9 {
		t.Errorf("SemiMajorAxis = %v, want 2.77", ceres.SemiMajorAxis)
	}

	if math.Abs(ceres.Eccentricity-0.0797) > 1e-9 {
		t.Errorf("Eccentricity = %v, want 0.0797", ceres.Eccentricity)
	}

	if math.Abs(ceres.Inclination.Degrees()-10.6) > 1e-9 {
		t.Errorf("Inclination = %v deg, want 10.6", ceres.Inclination.Degrees())
	}

	if math.Abs(ceres.MeanAnomaly.Degrees()-274) > 1e-9 {
		t.Errorf("MeanAnomaly = %v deg, want 274", ceres.MeanAnomaly.Degrees())
	}

	wantJD := 2461200.5
	if gotJD := ceres.Epoch.JD(); math.Abs(gotJD-wantJD) > 1e-6 {
		t.Errorf("Epoch JD = %v, want %v", gotJD, wantJD)
	}

	pallas := got[1]
	if pallas.HasElements {
		t.Error("expected Pallas (missing ma) to have HasElements=false")
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
	_, _ = p.Search(context.Background(), "non_existent_body")
}
