//go:build network
// +build network

package jpl

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

// requireHorizons skips the test when the JPL Horizons API is unreachable —
// per this project's network test policy, a reachability failure must
// never fail CI outright.
func requireHorizons(t *testing.T) {
	t.Helper()

	conn, err := net.DialTimeout("tcp", "ssd.jpl.nasa.gov:443", 5*time.Second)
	if err != nil {
		t.Skipf("JPL Horizons unreachable, skipping live test: %v", err)
	}

	_ = conn.Close()
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
		t.Fatalf("expected a resolved response, got error: %v", gotErr)
	}

	if len(got) != 1 {
		t.Fatalf("expected exactly 1 target for unambiguous query \"Ceres\", got %d: %+v", len(got), got)
	}

	if got[0].SPKID == "" {
		t.Errorf("expected a non-empty SPKID, got: %+v", got[0])
	}
}
