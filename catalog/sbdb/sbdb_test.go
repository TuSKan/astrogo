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
		name    string
		spkID   string
		isComet bool
		want    resolve.Kind
	}{
		{"Ceres", "20000001", false, resolve.KindDwarfPlanet},
		{"Pluto", "20134340", false, resolve.KindDwarfPlanet},
		{"Eris", "20136199", false, resolve.KindDwarfPlanet},
		{"Haumea", "20136108", false, resolve.KindDwarfPlanet},
		{"Makemake", "20136472", false, resolve.KindDwarfPlanet},
		{"Vesta", "20000004", false, resolve.KindAsteroid},
		{"ordinary comet", "1000036", true, resolve.KindComet},
		{"a comet SPK-ID that happens to match a dwarf-planet ID", "20000001", true, resolve.KindComet},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyKind(tt.spkID, tt.isComet); got != tt.want {
				t.Errorf("classifyKind(%q, %v) = %q, want %q", tt.spkID, tt.isComet, got, tt.want)
			}
		})
	}
}

func TestSearchBrightMock(t *testing.T) {
	asteroidJSON := `{
		"signature": {"version": "1.0"},
		"fields": ["full_name", "spkid", "H", "G"],
		"data": [
			["1 Ceres", "20000001", 3.34, 0.12],
			["4 Vesta", "20000004", 3.25, 0.32]
		],
		"count": 2
	}`
	cometJSON := `{
		"signature": {"version": "1.0"},
		"fields": ["full_name", "spkid", "M1", "K1"],
		"data": [
			["1P/Halley", "1000036", 4.5, 8.0]
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

	if len(got) != 3 {
		t.Fatalf("expected 3 targets (2 asteroids + 1 comet), got %d: %+v", len(got), got)
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

	halley := got[2]
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
