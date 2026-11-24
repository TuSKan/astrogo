//go:build network

package gaia

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/internal/testutil"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
)

func TestGaiaNetworkConeSearch(t *testing.T) {
	control := requireArchive(t, DefaultEndpoint)

	prov, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second) // ESA TAP can be slower
	defer cancel()

	// Tap the Pleiades core
	req := resolve.ConeRequest{
		Center: coord.NewICRS(angle.Deg(56.75), angle.Deg(24.116)),
		Radius: angle.Deg(0.05),
		Limit:  5,
	}

	iter := prov.ConeSearch(ctx, req)

	var targets []resolve.Target

	iter(func(tar resolve.Target, err error) bool {
		if err != nil {
			testutil.SkipOnDegradedService(t, err, control)
			t.Fatalf("Live network failed: %v", err)
		}

		targets = append(targets, tar)

		return true
	})

	if len(targets) == 0 {
		t.Fatalf("Expected stars from Gaia DR3 at Pleiades")
	}

	if !targets[0].HasCoord {
		t.Fatalf("Expected astremetry mapped to coordinates from Gaia")
	}
}

// A cone holding more sources than the limit returns the nearest of them,
// nearest first. Before #605 the query had no ORDER BY, and a TAP service
// returns the first rows it reaches: a capped cone came back as an arbitrary
// subset, as likely from the edge as from the center. The Trapezium sits in
// one of the densest patches of the Orion Nebula, so the five nearest of a
// 0.2° cone lie close to its center, far inside the edge.
func TestGaiaConeSearchReturnsTheNearestSources(t *testing.T) {
	control := requireArchive(t, DefaultEndpoint)

	prov, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	center := coord.NewICRS(angle.Deg(83.8221), angle.Deg(-5.3911))
	radius := angle.Deg(0.2)

	var seps []float64

	prov.ConeSearch(ctx, resolve.ConeRequest{Center: center, Radius: radius, Limit: 5})(func(tar resolve.Target, err error) bool {
		if err != nil {
			testutil.SkipOnDegradedService(t, err, control)
			t.Fatalf("ConeSearch: %v", err)
		}

		seps = append(seps, coord.Separation(center, tar.Coord).Degrees())

		return true
	})

	assertNearestFirst(t, seps, 5, radius.Degrees())
}

// assertNearestFirst checks a capped cone search returned want sources in
// order of distance, all well inside the cone: an arbitrary subset of a cone
// lies mostly toward its edge, where most of its area is.
func assertNearestFirst(t *testing.T, seps []float64, want int, radiusDeg float64) {
	t.Helper()

	if len(seps) != want {
		t.Fatalf("got %d sources, want %d", len(seps), want)
	}

	for i := 1; i < len(seps); i++ {
		if seps[i] < seps[i-1]-1e-9 {
			t.Errorf("source %d is %.5f° from the center, nearer than source %d at %.5f°; "+
				"the result should be nearest first", i, seps[i], i-1, seps[i-1])
		}
	}

	if far := seps[len(seps)-1]; far > radiusDeg/4 {
		t.Errorf("the farthest of the %d nearest sources is %.4f° out, in a %.2f° cone; "+
			"that is a subset of the cone, not its nearest sources", want, far, radiusDeg)
	}

	t.Logf("distances from the center: %.5f°", seps)
}

// TestGaiaResolvesTheSourcesSIMBADNames: the Gaia DR3 designations SIMBAD
// lists for Barnard's star and 61 Cyg A bring back exactly those two rows,
// each with its designation and its published errors.
func TestGaiaResolvesTheSourcesSIMBADNames(t *testing.T) {
	control := requireArchive(t, DefaultEndpoint)

	prov, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	want := []string{"Gaia DR3 1872046609345556480", "Gaia DR3 4472832130942575872"}
	ids := []resolve.Identifier{resolve.ParseIdentifier(want[0]), resolve.ParseIdentifier(want[1])}

	rows, err := resolve.Drain(prov.ResolveIdentifiers(ctx, ids), 0)
	if err != nil {
		testutil.SkipOnDegradedService(t, err, control)
		t.Fatalf("ResolveIdentifiers: %v", err)
	}

	got := make([]string, 0, len(rows))

	for _, r := range rows {
		got = append(got, r.Designation)

		if r.AstrometricError.RA <= 0 || r.AstrometricError.PmDec <= 0 {
			t.Errorf("%s carries no errors: %+v", r.Designation, r.AstrometricError)
		}
	}

	slices.Sort(got)

	if !slices.Equal(got, want) {
		t.Errorf("rows %v, want %v", got, want)
	}
}

// TestGaiaResolvesHipparcosAndTychoIdentifiers: a Hipparcos-2 or Tycho-2
// identifier brings back the source Gaia's cross-match gives as its best
// neighbor, carrying both identifiers: 61 Cyg A by HIP 104214, Barnard's
// star by TYC 425-2502-1. alf Cen A, too bright for Gaia, has no counterpart.
func TestGaiaResolvesHipparcosAndTychoIdentifiers(t *testing.T) {
	control := requireArchive(t, DefaultEndpoint)

	prov, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ids := []resolve.Identifier{
		resolve.ParseIdentifier("HIP 104214"), resolve.ParseIdentifier("TYC 425-2502-1"), resolve.ParseIdentifier("HIP 71683"),
	}

	rows, err := resolve.Drain(prov.ResolveIdentifiers(ctx, ids), 0)
	if err != nil {
		testutil.SkipOnDegradedService(t, err, control)
		t.Fatalf("ResolveIdentifiers: %v", err)
	}

	want := map[string][]string{
		"Gaia DR3 1872046609345556480": {"HIP 104214", "TYC 3168-2800-1"},
		"Gaia DR3 4472832130942575872": {"HIP 87937", "TYC 425-2502-1"},
	}

	if len(rows) != len(want) {
		t.Errorf("%d rows, want %d", len(rows), len(want))
	}

	for _, r := range rows {
		if !slices.Equal(r.Aliases, want[r.Designation]) {
			t.Errorf("%s: aliases %v, want %v", r.Designation, r.Aliases, want[r.Designation])
		}
	}
}
