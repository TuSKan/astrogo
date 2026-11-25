package coord_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

// eopRecordTail is a real finals2000A record's columns after its date and
// MJD. The parser slices by column, so records are built at its widths.
const eopRecordTail = " I  0.120733 0.009786  0.136966 0.015902  " +
	"I 0.8084178 0.0002710  0.0000 0.1916  P    -0.766    0.199    -0.720    0.300"

// bulletinFrom is n consecutive finals2000A records from MJD start.
func bulletinFrom(start, n int) string {
	var s strings.Builder

	for i := range n {
		mjd := start + i
		t := epoch.FromJD(float64(mjd)+2400000.5, epoch.UTC)
		fmt.Fprintf(&s, "%2d%2d%2d %8.2f%s\n", t.Year()%100, int(t.Month()), t.Day(), float64(mjd), eopRecordTail)
	}

	return s.String()
}

// TestContextReportsTheEOPItWasPlacedWith: a Context says which Earth
// orientation its instant was placed with, as epoch.EOPSource names it, and
// whether the instant is inside the loaded bulletin (#1187). With nothing
// loaded it is "zero" and outside; with a bulletin LoadEOP fetched it is
// "network", inside for an instant the bulletin covers and outside once
// SetTime moves past its end; with a model chosen by RegisterModel it is
// "explicit".
func TestContextReportsTheEOPItWasPlacedWith(t *testing.T) {
	// Not parallel: the EOP model and remote's configuration are process-wide.
	t.Cleanup(remote.Capture().Restore)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	// The bulletin's source is a local directory, empty until the bulletin
	// is written below, so the lazy load first finds nothing.
	src := testutil.FileURL(t, t.TempDir())
	if err := remote.SetURL(remote.IERSFinals2000A, src); err != nil {
		t.Fatal(err)
	}

	keepEOP(t)
	epoch.ResetEOP()

	site := coord.MustGeodetic(angle.LatDeg(-24.62722), angle.LonDeg(-70.40417), 2635)
	inside := epoch.FromJD(61050.5+2400000.5, epoch.UTC) // 2026-01-10
	past := epoch.FromJD(61400.5+2400000.5, epoch.UTC)

	check := func(name string, ctx *coord.Context, wantSource string, wantCovered bool) {
		t.Helper()

		if source, covered := ctx.EOPSource(); source != wantSource || covered != wantCovered {
			t.Errorf("%s: EOPSource = %q, %v, want %q, %v", name, source, covered, wantSource, wantCovered)
		}
	}

	check("nothing loaded", mustContext(t, inside, site, atmosphere.Refraction{}), "zero", false)

	fsys, err := remote.OpenFS(t.Context(), src)
	if err != nil {
		t.Fatalf("open the source: %v", err)
	}

	if err := remote.WriteFile(t.Context(), fsys, "finals2000A.all", strings.NewReader(bulletinFrom(61041, 30))); err != nil {
		t.Fatalf("seed the source: %v", err)
	}

	if err := epoch.LoadEOP(t.Context()); err != nil {
		t.Fatalf("LoadEOP: %v", err)
	}

	ctx := mustContext(t, inside, site, atmosphere.Refraction{})
	check("inside the bulletin", ctx, "network", true)

	ctx.SetTime(past)
	check("moved past the bulletin", ctx, "network", false)

	epoch.RegisterModel(epoch.ZeroModel{})
	check("an explicit model", mustContext(t, inside, site, atmosphere.Refraction{}), "explicit", false)
}
