package coord_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// eopRecordTail is a real finals2000A record's columns after its date and
// MJD. The parser slices by column, so records are built at its widths.
const eopRecordTail = " I  0.120733 0.009786  0.136966 0.015902  " +
	"I 0.8084178 0.0002710  0.0000 0.1916  P    -0.766    0.199    -0.720    0.300"

// fetchedBulletin is an EOP loader whose network step answers n consecutive
// records from MJD start, and whose cache holds nothing.
type fetchedBulletin struct{ start, n int }

func (b fetchedBulletin) Fetch(context.Context) (time.EOPData, error) {
	var s strings.Builder

	for i := range b.n {
		mjd := b.start + i
		t := time.FromJD(float64(mjd)+2400000.5, time.UTC)
		fmt.Fprintf(&s, "%2d%2d%2d %8.2f%s\n", t.Year()%100, int(t.Month()), t.Day(), float64(mjd), eopRecordTail)
	}

	return time.EOPData{Raw: []byte(s.String())}, nil
}

func (fetchedBulletin) Cached(context.Context) (time.EOPData, error) {
	return time.EOPData{}, time.ErrNoEOPData
}

// TestContextReportsTheEOPItWasPlacedWith: a Context says which Earth
// orientation its instant was placed with, as time.EOPSource names it, and
// whether the instant is inside the loaded bulletin (#1187). With nothing
// loaded it is "zero" and outside; with a bulletin LoadEOP fetched it is
// "network", inside for an instant the bulletin covers and outside once
// SetTime moves past its end; with a model chosen by RegisterModel it is
// "explicit".
func TestContextReportsTheEOPItWasPlacedWith(t *testing.T) {
	// Not parallel: the EOP model and loader are process-wide.
	time.RegisterEOPLoader(nil)
	time.ResetEOP()

	t.Cleanup(func() {
		time.RegisterEOPLoader(nil)
		time.ResetEOP()
	})

	site := coord.MustGeodetic(angle.Deg(-70.40417), angle.Deg(-24.62722), 2635)
	inside := time.FromJD(61050.5+2400000.5, time.UTC) // 2026-01-10
	past := time.FromJD(61400.5+2400000.5, time.UTC)

	check := func(name string, ctx *coord.Context, wantSource string, wantCovered bool) {
		t.Helper()

		if source, covered := ctx.EOPSource(); source != wantSource || covered != wantCovered {
			t.Errorf("%s: EOPSource = %q, %v, want %q, %v", name, source, covered, wantSource, wantCovered)
		}
	}

	check("nothing loaded", coord.NewContext(inside, site, atmosphere.Refraction{}), "zero", false)

	time.RegisterEOPLoader(fetchedBulletin{start: 61041, n: 30})

	if err := time.LoadEOP(t.Context()); err != nil {
		t.Fatalf("LoadEOP: %v", err)
	}

	ctx := coord.NewContext(inside, site, atmosphere.Refraction{})
	check("inside the bulletin", ctx, "network", true)

	ctx.SetTime(past)
	check("moved past the bulletin", ctx, "network", false)

	time.RegisterModel(time.ZeroModel{})
	check("an explicit model", coord.NewContext(inside, site, atmosphere.Refraction{}), "explicit", false)
}
