package mast

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/catalog/xmatch"
	"github.com/TuSKan/astrogo/internal/testutil"

	"github.com/TuSKan/astrogo/remote"
)

func TestMastOfflineResolve(t *testing.T) {
	jsonPayload := `{
	"resolvedCoordinate": [{
		"resolver": "NED",
		"ra": 10.684,
		"decl": 41.269,
		"canonicalName": "M31"
	}],
	"status": "COMPLETE"
}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if _, err := fmt.Fprint(w, jsonPayload); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	prov := New()

	redirect(t, server.URL)

	req := resolve.ObjectRequest{Query: "M31"}
	iter := prov.ResolveObject(context.Background(), req)

	var targets []resolve.Target

	iter(func(tar resolve.Target, err error) bool {
		testutil.AssertNoError(t, err)

		targets = append(targets, tar)

		return true
	})

	if len(targets) != 1 {
		t.Fatalf("Expected 1 target, got %d", len(targets))
	}

	testutil.AssertEqual(t, "Name", targets[0].Name, "M31")
	// Catalog is always "mast" (consistent with every other provider setting
	// Catalog to its own name), never the relayed sub-resolver name — see
	// newMASTTarget's doc comment. Nor is that name an alias: aliases name the
	// object, and matching on "NED" would make every NED-answered target the
	// same object (#612).
	testutil.AssertEqual(t, "Catalog", targets[0].Catalog, "mast")

	if len(targets[0].Aliases) != 0 {
		t.Errorf("Aliases = %v; the relayed resolver's name is not a name of the object", targets[0].Aliases)
	}

	testutil.AssertEqual(t, "RA", targets[0].Coord.RA().Degrees(), 10.684)

	if targets[0].Epoch.IsZero() {
		t.Error("expected a default J2000 Epoch, got zero value")
	}
}

// TestMastOfflineResolveXML covers a live-observed MAST quirk: the invoke
// API sometimes ignores the request's "format": "json" field and returns
// its default XML body anyway. ResolveObject must sniff and decode this
// correctly rather than failing to parse it as JSON.
func TestMastOfflineResolveXML(t *testing.T) {
	xmlPayload := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><ns2:resolvedItems xmlns:ns2="santa.stsci.edu"><resolvedCoordinate><canonicalName>M  31</canonicalName><dec>41.26875</dec><objectType>AGN</objectType><ra>10.684708</ra><resolver>SIMBAD</resolver><resolverTime>513</resolverTime><searchString>m31</searchString></resolvedCoordinate><status></status></ns2:resolvedItems>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/xml;charset=UTF-8")

		if _, err := fmt.Fprint(w, xmlPayload); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	prov := New()

	redirect(t, server.URL)

	req := resolve.ObjectRequest{Query: "M31"}
	iter := prov.ResolveObject(context.Background(), req)

	var targets []resolve.Target

	iter(func(tar resolve.Target, err error) bool {
		testutil.AssertNoError(t, err)

		targets = append(targets, tar)

		return true
	})

	if len(targets) != 1 {
		t.Fatalf("Expected 1 target, got %d", len(targets))
	}

	testutil.AssertEqual(t, "Name", targets[0].Name, "M  31")
	testutil.AssertEqual(t, "Catalog", targets[0].Catalog, "mast")

	if len(targets[0].Aliases) != 0 {
		t.Errorf("Aliases = %v; the relayed resolver's name is not a name of the object", targets[0].Aliases)
	}

	testutil.AssertEqual(t, "RA", targets[0].Coord.RA().Degrees(), 10.684708)
	testutil.AssertEqual(t, "Dec", targets[0].Coord.Dec().Degrees(), 41.26875)
}

// TestMastOfflineResolveXMLNoMatch covers the XML "not found" shape: no
// resolvedCoordinate element and an empty status, as observed live for an
// unresolvable name — this must yield zero targets, not an error.
func TestMastOfflineResolveXMLNoMatch(t *testing.T) {
	xmlPayload := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><ns2:resolvedItems xmlns:ns2="santa.stsci.edu"><status></status></ns2:resolvedItems>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/xml;charset=UTF-8")

		if _, err := fmt.Fprint(w, xmlPayload); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	prov := New()

	redirect(t, server.URL)

	req := resolve.ObjectRequest{Query: "ThisIsNotARealObjectXYZ123"}
	iter := prov.ResolveObject(context.Background(), req)

	var targets []resolve.Target

	iter(func(tar resolve.Target, err error) bool {
		testutil.AssertNoError(t, err)

		targets = append(targets, tar)

		return true
	})

	if len(targets) != 0 {
		t.Fatalf("Expected 0 targets, got %d", len(targets))
	}
}

// TestNewMASTTarget_MissingCoordIsNotFake is a regression test: a match with
// no ra/dec (nil pointers, e.g. an XML response with no <ra>/<dec> element)
// must yield HasCoord=false, never a fake (0,0) reported as real — the same
// bug class fixed in Gaia's row parsing.
func TestNewMASTTarget_MissingCoordIsNotFake(t *testing.T) {
	got := newMASTTarget("M31", nil, nil)

	if got.HasCoord {
		t.Errorf("expected HasCoord=false for a match with no coordinate, got HasCoord=true Coord=%v", got.Coord)
	}

	if got.Catalog != "mast" {
		t.Errorf("Catalog = %q, want mast", got.Catalog)
	}

	if len(got.Aliases) != 0 {
		t.Errorf("Aliases = %v, want none", got.Aliases)
	}

	if got.Epoch.IsZero() {
		t.Error("expected a default J2000 Epoch even without a coordinate")
	}
}

func TestProviderInterface(t *testing.T) {
	p := New()
	if p.Name() != "mast" {
		t.Errorf("expected mast, got %s", p.Name())
	}

	caps := p.Capabilities()
	if len(caps) != 2 || caps[0] != resolve.CapObjectResolution || caps[1] != resolve.CapConeSearch {
		t.Errorf("expected CapObjectResolution and CapConeSearch, got %v", caps)
	}

	// errTransport keeps this default (non-network-tagged) test fully
	// offline — see CLAUDE.md's build-tag convention.
	redirect(t, "http://127.0.0.1:1")

	_, _ = p.Resolve(context.Background(), "non_existent_body")
	_, _ = p.Search(context.Background(), "non_existent_body")
}

// TestConeSearchNotImplemented confirms ConeSearch surfaces ErrNotImplemented
// explicitly rather than a fabricated empty-but-successful result — it
// advertises resolve.CapConeSearch but CAOM spatial search isn't implemented
// yet (see doc comment on ConeSearch).
func TestConeSearchNotImplemented(t *testing.T) {
	p := New()

	iter := p.ConeSearch(context.Background(), resolve.ConeRequest{})
	iter(func(_ resolve.Target, err error) bool {
		if !errors.Is(err, ErrNotImplemented) {
			t.Errorf("expected ErrNotImplemented, got %v", err)
		}

		return false
	})
}

// redirect points endpoint id at a test server for the duration of one
// test. It replaces the old http.RoundTripper injection: remote/api's
// Client is opaque by design, and every request resolves its URL through
// remote.URL(id) anyway, so the registry is the natural seam.
func redirect(t *testing.T, url string) {
	t.Helper()

	id := remote.MAST

	scope := remote.Capture(id)
	t.Cleanup(scope.Restore)

	if err := remote.SetURL(id, url); err != nil {
		t.Fatalf("SetURL(%s): %v", id, err)
	}
}

// Two MAST results for different objects, both answered by the same relayed
// resolver, are not one object. With the resolver's name stored as an alias
// they were: xmatch's alias pass, which needs no position, paired M31 with M33
// and with Vega, 30 and more degrees apart (#612).
func TestMASTTargetsFromOneResolverDoNotCrossMatch(t *testing.T) {
	t.Parallel()

	ra31, dec31 := 10.684708, 41.26875
	ra33, dec33 := 23.462042, 30.660222
	raVega, decVega := 279.234735, 38.783689

	m31 := newMASTTarget("M  31", &ra31, &dec31)
	m33 := newMASTTarget("M  33", &ra33, &dec33)
	vega := newMASTTarget("* alf Lyr", &raVega, &decVega)

	if pairs := xmatch.Match([]resolve.Target{m31, m33}, []resolve.Target{vega}); len(pairs) != 0 {
		t.Errorf("xmatch paired %d MAST targets for different objects: %v", len(pairs), pairs)
	}

	if pairs := xmatch.Match([]resolve.Target{m31}, []resolve.Target{m33}); len(pairs) != 0 {
		t.Errorf("xmatch paired M31 with M33: %v", pairs)
	}
}
