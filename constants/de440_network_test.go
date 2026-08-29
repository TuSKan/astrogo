//go:build network

package constants_test

import (
	"bufio"
	"context"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

// kernelAssignment matches a NAIF text-kernel line such as
//
//	BODY499_GM     = ( 4.282837362069909E+04  )
//
// The exponent marker is D or E: the kernel format descends from Fortran and
// uses both, sometimes within one file — BODY8_GM is written with D while
// BODY7_GM beside it uses E.
var kernelAssignment = regexp.MustCompile(
	`^\s*BODY(\d+)_GM\s*=\s*\(\s*([0-9.+-]+)[DdEe]([+-]?\d+)\s*\)`)

// TestDE440MatchesNAIF re-fetches the kernel and checks every value in
// [constants.DE440] against it.
//
// The constants are transcribed by hand, because a constants table has to be
// available at compile time and offline. Transcription is where a digit gets
// dropped, and a mass parameter wrong in its eleventh digit produces orbits
// that look entirely reasonable — so the transcription is checked against the
// source rather than trusted.
//
// It also checks the unit conversion. NAIF publishes km³/s² and this package
// states m³/s², which is a shift of the decimal exponent by nine; that is
// exactly the kind of change that is obviously right and occasionally typed
// wrong.
func TestDE440MatchesNAIF(t *testing.T) {
	testutil.RequireReachable(t, "naif.jpl.nasa.gov:443")

	remote.EnableDownloads(0, remote.NAIFPCK)

	ctx := context.Background()

	bucket, key, err := remote.GetFile(ctx, remote.NAIFPCK, "pck/gm_de440.tpc")
	if err != nil {
		testutil.SkipOnUpstreamFailure(t, err)
		t.Fatalf("fetch gm_de440.tpc: %v", err)
	}

	raw, err := bucket.ReadAll(ctx, key)
	if err != nil {
		t.Fatalf("read gm_de440.tpc: %v", err)
	}

	published := parseGM(t, string(raw))
	if len(published) < 20 {
		t.Fatalf("parsed only %d GM assignments; the kernel format may have changed", len(published))
	}

	// NAIF body code for each constant this package states.
	cases := []struct {
		body int
		got  constants.Constant
	}{
		{10, constants.DE440.SunGravitationalParameter},
		{1, constants.DE440.MercurySystemGravitationalParameter},
		{2, constants.DE440.VenusSystemGravitationalParameter},
		{3, constants.DE440.EarthMoonGravitationalParameter},
		{4, constants.DE440.MarsSystemGravitationalParameter},
		{5, constants.DE440.JupiterSystemGravitationalParameter},
		{6, constants.DE440.SaturnSystemGravitationalParameter},
		{7, constants.DE440.UranusSystemGravitationalParameter},
		{8, constants.DE440.NeptuneSystemGravitationalParameter},
		{9, constants.DE440.PlutoSystemGravitationalParameter},
		{399, constants.DE440.EarthGravitationalParameter},
		{301, constants.DE440.MoonGravitationalParameter},
		{499, constants.DE440.MarsGravitationalParameter},
		{599, constants.DE440.JupiterGravitationalParameter},
		{699, constants.DE440.SaturnGravitationalParameter},
		{799, constants.DE440.UranusGravitationalParameter},
		{899, constants.DE440.NeptuneGravitationalParameter},
		{999, constants.DE440.PlutoGravitationalParameter},
	}

	if len(cases) != len(constants.DE440.All()) {
		t.Errorf("%d constants in the set but %d checked here; a new member needs a body code",
			len(constants.DE440.All()), len(cases))
	}

	for _, c := range cases {
		km, ok := published[c.body]
		if !ok {
			t.Errorf("BODY%d_GM is not in the kernel", c.body)

			continue
		}

		// km³/s² to m³/s². Compared relatively: the two differ only by how
		// the same decimal digits are parsed, so anything above a few ULP
		// is a transcription error rather than arithmetic.
		want := km * 1e9
		if rel := relDiff(c.got.Value, want); rel > 1e-15 {
			t.Errorf("%s (BODY%d): have %.17g, kernel has %.17g m3/s2 (relative %.3g)",
				c.got.Symbol, c.body, c.got.Value, want, rel)
		}
	}
}

// TestDE440SystemExceedsBody is the distinction the set exists to make: a
// system parameter includes the satellites and must therefore be the larger
// of the two.
//
// It is checked against the kernel rather than against this package's own
// values, so a pair of constants transcribed into each other's fields fails
// here as well as above.
func TestDE440SystemExceedsBody(t *testing.T) {
	testutil.RequireReachable(t, "naif.jpl.nasa.gov:443")

	remote.EnableDownloads(0, remote.NAIFPCK)

	ctx := context.Background()

	bucket, key, err := remote.GetFile(ctx, remote.NAIFPCK, "pck/gm_de440.tpc")
	if err != nil {
		testutil.SkipOnUpstreamFailure(t, err)
		t.Fatalf("fetch gm_de440.tpc: %v", err)
	}

	raw, err := bucket.ReadAll(ctx, key)
	if err != nil {
		t.Fatalf("read gm_de440.tpc: %v", err)
	}

	published := parseGM(t, string(raw))

	for _, p := range []struct {
		name           string
		system, planet int
	}{
		{"Mars", 4, 499},
		{"Jupiter", 5, 599},
		{"Saturn", 6, 699},
		{"Uranus", 7, 799},
		{"Neptune", 8, 899},
		{"Pluto", 9, 999},
	} {
		sys, planet := published[p.system], published[p.planet]
		if sys <= planet {
			t.Errorf("%s: system GM %.17g is not greater than the planet's %.17g", p.name, sys, planet)
		}

		t.Logf("%-8s system exceeds body by %.6g km3/s2 (1 part in %.0f)",
			p.name, sys-planet, sys/(sys-planet))
	}
}

// parseGM extracts every BODYnnn_GM assignment, in km³/s².
func parseGM(t *testing.T, src string) map[int]float64 {
	t.Helper()

	out := make(map[int]float64)

	sc := bufio.NewScanner(strings.NewReader(src))
	for sc.Scan() {
		m := kernelAssignment.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}

		body, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}

		// Rebuild with an E marker so Go's parser accepts the Fortran form.
		v, err := strconv.ParseFloat(m[2]+"e"+m[3], 64)
		if err != nil {
			t.Errorf("BODY%d_GM: %v", body, err)

			continue
		}

		out[body] = v
	}

	if err := sc.Err(); err != nil {
		t.Fatalf("scan kernel: %v", err)
	}

	return out
}

func relDiff(a, b float64) float64 {
	if b == 0 {
		return 0
	}

	d := (a - b) / b
	if d < 0 {
		return -d
	}

	return d
}
