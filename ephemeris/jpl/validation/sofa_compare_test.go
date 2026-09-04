//go:build validation

package jpl_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/internal/metrology"
	"github.com/TuSKan/astrogo/time"
)

// sofaEpoch is one fixed instant in the comparison, named so a failing
// sample identifies itself.
//
// Fixed, because a validation result whose epoch is "now" cannot be
// reproduced: this test previously used time.NowUTC() for one of its three
// points, so every run compared a different instant and any failure would
// have been unreproducible by the person reading the log. It also meant the
// contract below was re-rolled against a fresh epoch on every run — see
// contractFor for why that mattered more than it looks.
//
// Every epoch sits inside both reference routines' documented validity
// windows — Epv00 quotes 1900-2100 and Moon98 1950-2100 — and inside the era
// in which the comparison measures an ephemeris rather than a clock. See
// [sofaLastYear] and [firstLeapSecondYear].
type sofaEpoch struct {
	name string
	t    time.Time
}

// sofaLastYear ends the window at both reference routines' own upper limit.
// Epv00 quotes its accuracy over 1900-2100 and Moon98 over 1950-2100, so
// sampling past 2100 would measure against a bound neither offers there.
//
// The window's lower end is not the routines' claim but the clock's, and is
// the same for both: [firstLeapSecondYear]. Before it the kernel path and the
// analytical path reach a TDB instant by different routes and disagree, and
// that disagreement swamps both routines — the Sun's residual is 7.9 km in
// 1972 and 302.3 km in 1971. Sampling Epv00 from its documented 1900 measures
// the leap-second boundary rather than the ephemeris: the first run of this
// widened window did exactly that and reported a maximum 47x over contract,
// with every offending sample in 1900. See
// TestNoTimekeepingStepAtTheLeapSecondBoundary.
const sofaLastYear = 2100

// sofaEpochs samples the window quarterly.
//
// It used to be three named instants, which is not enough to say anything
// about a maximum — and one of the three used to be time.NowUTC(), so the
// contract's margin was re-rolled on every run and a failure could not be
// reproduced by the person reading the log. Fixing the wall clock removed the
// irreproducibility; this removes the sample size, and brings these two rows
// onto the same footing as the planets in sofa_planets_test.go, which would
// otherwise sit beside them in the generated table with four hundred times
// the evidence.
func sofaEpochs() []sofaEpoch {
	first, last := firstLeapSecondYear, sofaLastYear

	quarters := []time.Month{time.January, time.April, time.July, time.October}

	out := make([]sofaEpoch, 0, (last-first+1)*len(quarters))

	for y := first; y <= last; y++ {
		for _, mo := range quarters {
			at := time.Date(y, mo, 1, 0, 0, 0, 0, time.LocationUTC)
			out = append(out, sofaEpoch{name: at.Format(isoDate), t: at})
		}
	}

	return out
}

// contractFor is the agreement bound for one body.
//
// # Why these numbers changed, and why the old ones were unsound
//
// This comparison is DE440 against an analytical series, so the bound that
// means anything is the *series'* own published accuracy — exceeding it says
// astrogo mis-evaluates the series or the kernel, while sitting inside it
// says nothing is wrong. gofa documents both routines directly:
//
//   - Epv00 (the Sun path), compared with JPL DE405 over 1900-2100:
//     heliocentric position error RMS 3.7 km, max 11.2 km — 7.49e-08 AU.
//   - Moon98, compared with ELP/MPP02 over 1950-2100: RMS 6.1 km,
//     worst case 31.7 km — 2.12e-07 AU.
//
// The previous tolerances were 1e-6 AU for the Sun and 1e-7 AU for the Moon,
// and both were wrong in opposite directions. 1e-6 AU is 149.6 km, thirteen
// times Epv00's published worst case, so the Sun bound could not have caught
// a real regression. 1e-7 AU is 15.0 km — **less than half** Moon98's
// published worst case of 31.7 km, so the Moon bound demanded the two agree
// twice as closely as SOFA documents its own routine to be accurate. It
// passed only because the sampled epochs happened to land in a favourable
// part of the lunar residual, and one of those epochs was time.NowUTC(), so
// that luck was re-rolled on every single run.
//
// The bounds below are the published worst cases doubled. The factor is not
// margin-for-comfort: the quoted figures are against DE405 and ELP/MPP02
// respectively, while this test compares against DE440, and the difference
// between those reference ephemerides is itself nonzero. Doubling covers it
// and keeps the bound a genuine scientific limit rather than a transcription
// of whatever this repository last measured.
func contractFor(bid eph.ID) metrology.Contract {
	if bid == eph.Moon {
		return metrology.MustContract(4.0e-7, "AU",
			"2x Moon98's published worst case against ELP/MPP02, 31.7 km over 1950-2100, "+
				"doubled because that figure is against ELP/MPP02 while this compares against DE440",
			"gofa ephem.go, Moon98 note 3")
	}

	return metrology.MustContract(1.5e-7, "AU",
		"2x Epv00's published max heliocentric position error against DE405, 11.2 km over "+
			"1900-2100, doubled because that figure is against DE405 while this compares against DE440",
		"gofa ephem.go, Epv00 note 4")
}

// sofaReference records what this is compared against, and why the agreement
// is weaker evidence than a tick would suggest.
//
// Both sides run through astrogo. One reads a JPL kernel and the other
// evaluates gofa's analytical series, so the *models* are genuinely
// independent — but the series is astrogo's own dependency rather than a
// third party's implementation, so a fault in how astrogo drives gofa is
// invisible here and only a fault in how it drives the kernel would show.
// Recording the shared ancestry makes the generated table say so instead of
// leaving a reader to work it out.
func sofaReference() metrology.Reference {
	return metrology.Reference{
		Kind:           metrology.KindSOFA,
		Name:           "gofa (Epv00 / Moon98)",
		Version:        "v1.19.1",
		Source:         "github.com/hebl/gofa, SOFA-derived",
		Dataset:        "compared against DE440",
		SharedAncestor: "SOFA",
	}
}

func runSOFATest(t *testing.T, bid eph.ID) {
	t.Helper()

	suite := metrology.NewSuite("ephemeris.sofa."+strings.ToLower(bid.String()),
		sofaReference(), contractFor(bid))

	p, err := jpl.NewProvider(context.Background(), core.Planets, "de440")
	if err != nil {
		metrology.NotVerified(t, "the JPL provider could not be built: "+err.Error(), suite)
	}

	defer func() { _ = p.Close() }()

	sofa := eph.Default()

	for _, e := range sofaEpochs() {
		jplState, err := p.State(bid, e.t)
		if err != nil {
			t.Fatalf("JPL State() failed at %s: %v", e.name, err)
		}

		sofaState, err := sofa.State(bid, e.t)
		if err != nil {
			t.Fatalf("SOFA State() failed at %s: %v", e.name, err)
		}

		posDiff := jplState.Pos.Sub(sofaState.Pos).Norm()

		suite.Add(metrology.Sample{
			Error:   posDiff,
			Label:   bid.String() + " @ " + e.name,
			Context: fmt.Sprintf("%s, |DE440 - SOFA| = %.2f km", e.name, posDiff*kmPerAU),
		})
	}

	suite.Report(t)
}

func TestJPLStateAgainstSOFASun(t *testing.T) {
	runSOFATest(t, eph.Sun)
}

func TestJPLStateAgainstSOFAMoon(t *testing.T) {
	runSOFATest(t, eph.Moon)
}
