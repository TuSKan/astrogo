//go:build network

package jpl

import (
	"context"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/time"
)

// requireHorizons skips the test when the JPL Horizons API is unreachable —
// per this project's network test policy, a reachability failure must
// never fail CI outright.
func requireHorizons(t *testing.T) {
	t.Helper()

	testutil.RequireReachable(t, "ssd.jpl.nasa.gov:443")
}

// TestJPLNetworkResolve confirms the provider reaches the live Horizons API
// and resolves an ambiguous major-body query ("Mars" matches the planet,
// its barycenter, and several spacecraft) into real resolve.Targets.
func TestJPLNetworkResolve(t *testing.T) {
	requireHorizons(t)

	prov := New()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := resolve.ObjectRequest{Query: "Mars"}
	iter := prov.ResolveObject(ctx, req)

	var (
		got    []resolve.Target
		gotErr error
	)

	iter(func(t resolve.Target, err error) bool {
		if err != nil {
			gotErr = err
			return false
		}

		got = append(got, t)

		return true
	})

	if gotErr != nil {
		testutil.SkipOnUpstreamFailure(t, gotErr)
		t.Fatalf("expected a resolved response, got error: %v", gotErr)
	}

	if len(got) == 0 {
		t.Fatal("expected at least one match for ambiguous query \"Mars\"")
	}

	found := false

	for _, tg := range got {
		if tg.Name == "Mars" {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected a target named \"Mars\" among matches, got: %+v", got)
	}
}

// TestJPLNetworkResolveExact confirms an unambiguous small-body query
// resolves to exactly one Target via the "Target body name:" header parse.
func TestJPLNetworkResolveExact(t *testing.T) {
	requireHorizons(t)

	prov := New()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := resolve.ObjectRequest{Query: "Ceres"}
	iter := prov.ResolveObject(ctx, req)

	var (
		got    []resolve.Target
		gotErr error
	)

	iter(func(t resolve.Target, err error) bool {
		if err != nil {
			gotErr = err
			return false
		}

		got = append(got, t)

		return true
	})

	if gotErr != nil {
		testutil.SkipOnUpstreamFailure(t, gotErr)
		t.Fatalf("expected a resolved response, got error: %v", gotErr)
	}

	if len(got) != 1 {
		t.Fatalf("expected exactly 1 target for unambiguous query \"Ceres\", got %d: %+v", len(got), got)
	}

	if got[0].SPKID == "" {
		t.Errorf("expected a non-empty SPKID, got: %+v", got[0])
	}
}

// TestJPLNetworkResolvesCommonNames resolves names Horizons answers with an
// ambiguous table, a spacecraft header with two parentheticals, and a
// four-column small-body index. Before #616 these came back as Larissa, the
// Earth-Moon barycenter, the ID "spacecraft" and nothing.
func TestJPLNetworkResolvesCommonNames(t *testing.T) {
	requireHorizons(t)

	tests := []struct {
		query, wantID, wantName string
	}{
		{"ISS", "-125544", "International Space Station (spacec"},
		{"Moon", "301", "Moon"},
		{"Voyager 1", "-31", "Voyager 1 (spacecraft)"},
		{"Halley", "2688", "Halley"},
		// Asteroids whose names occur inside a major body's: Horizons
		// answered Kerberos, Thebe and OSIRIS-REx until Search asked the
		// small bodies again (#618).
		{"Eros", "A898 PA", "433 Eros"},
		{"Hebe", "A847 NA", "6 Hebe"},
		{"Iris", "A847 PA", "7 Iris"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			got, err := New().Resolve(ctx, tt.query)
			if err != nil {
				testutil.SkipOnUpstreamFailure(t, err)
				t.Fatalf("Resolve(%q): %v", tt.query, err)
			}

			testutil.AssertEqual(t, "ID", got.ID, tt.wantID)
			testutil.AssertEqual(t, "Name", got.Name, tt.wantName)
		})
	}
}
