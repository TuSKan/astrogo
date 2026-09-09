//go:build network

package jpl_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/internal/metrology"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// horizonsCase pairs a Horizons target with the astrogo body it is compared
// against, so the NAIF id, the display name and the eph.ID travel together
// instead of being re-derived from the name by an if-chain further down.
type horizonsCase struct {
	naifID int
	name   string
	id     eph.ID
}

func horizonsCases() []horizonsCase {
	return []horizonsCase{
		{10, "Sun", eph.Sun},
		{301, "Moon", eph.Moon},
		{4, "Mars", eph.Mars},
	}
}

// loadCases fetches every comparison point in one pass.
//
// It used to be called once per top-level test, each of which fetched all
// three bodies and then discarded the two it did not want — nine Horizons
// queries to use three. Since the point of the network tag is that these
// tests talk to somebody else's server, asking for the same three vectors
// three times is not a style question but a courtesy one.
func loadCases(t *testing.T) []*StateVector {
	t.Helper()

	// The TDB suffix is required, not decorative: fetchVector sends
	// TIME_TYPE='UT', under which Horizons labels its time column JDUT, and
	// the conversion there reads it as TDB. See fetchVector's doc comment.
	const (
		start = "2000-01-01 12:00 TDB"
		stop  = "2000-01-01 12:01"
	)

	out := make([]*StateVector, 0, len(horizonsCases()))

	for _, c := range horizonsCases() {
		sv, err := fetchVector(strconv.Itoa(c.naifID), c.name, start, stop)
		if errors.Is(err, errHorizonsUnavailable) {
			return nil
		}

		if err != nil {
			t.Fatalf("failed to fetch %s vector: %v", c.name, err)
		}

		out = append(out, sv)
	}

	return out
}

// horizonsStateReference is what this compares against.
//
// Horizons serves DE441 and the local provider reads DE440. Both are JPL
// ephemerides, so this is a consistency check on astrogo's SPK evaluation and
// time-scale handling rather than independent validation of the ephemeris —
// and they are not even the same JPL ephemeris, which is worth more than the
// usual shared-ancestry note: the two kernels differ by 2.42 m at the Moon,
// so part of what this measures is the gap between them rather than anything
// astrogo does. SharedAncestor records both facts so the generated table
// states them instead of leaving a reader to infer either.
func horizonsStateReference() metrology.Reference {
	return metrology.Reference{
		Kind:           metrology.KindHorizons,
		Name:           "JPL Horizons",
		Version:        "VECTORS, geocentric, ICRF",
		Source:         "https://ssd.jpl.nasa.gov/api/horizons.api",
		Dataset:        "DE441 (Horizons) against DE440 (local)",
		SharedAncestor: "JPL DE — and not the same kernel: DE440/DE441 differ by 2.42 m at the Moon",
	}
}

// TestJPLStateAgainstHorizons compares astrogo's DE440 evaluation against
// Horizons' own state vectors for the same instant.
//
// # Why the tolerances moved by five orders of magnitude
//
// They were 1e-7 AU and 1e-8 AU/day, with no stated reason — about a hundred
// thousand times the largest real residual, which is a bound that cannot fail
// for the reason it exists. docs/VALIDATION.md published the measured figures
// as the achieved accuracy of astrogo's ephemerides, which they never were.
//
// # The two sides do not read the same kernel, and the bound accounts for it
//
// This comment used to say "both sides evaluate the same JPL integration —
// DE441 and DE440 share it over this span", and derived the bound from that.
// It is not true. Horizons reports {source: DE441} for these queries while
// this test reads de440, and measured directly, geocentric positions from the
// two kernels differ by 2.94 cm for the Sun, Mars and Jupiter and by 2.42 m
// for the Moon — at J2000.0 and across 2026 alike (#255).
//
// So the residual has three terms, not two, and they separate cleanly by size:
//
//	astrogo's SPK evaluation and time scales   what this test is for
//	the DE440/DE441 difference                 1.6e-11 AU at the Moon
//	the cheapest fault worth catching          6.7e-9 AU, one second of UT
//
// The bound of 1e-9 AU sits above the kernel difference by sixty times and
// below the fault threshold by seven, which is what makes it a usable
// discriminator rather than a number pinned to a measurement. A one-second
// time-scale error — which this repository had until recently in its
// leap-second parsing — moves the Moon about a kilometre and cannot hide
// under it.
//
// Most of the Moon's measured residual is therefore the kernel difference and
// not astrogo. That is stated here, and in docs/VALIDATION.md, so the figure
// is not read as astrogo's achieved accuracy — which is exactly the error the
// old tolerances were fixed for.
//
// # Why de440 rather than the ephemeris Horizons serves
//
// Measured, switching to de441_part-2 takes the position from max 4.52e-12 AU
// (0.676 m, worst body the Moon) to 9.80e-13 AU (0.147 m, worst body Mars),
// and the velocity from 1.006e-12 to 1.405e-14 AU/day — 4.6 and 72 times
// closer, with the Moon's excess gone. It would make the residual
// attributable to astrogo alone.
//
// It is not worth what it costs. de441_part-2 is 1580 MB against de440's 114
// MB, this package's TestMain grants unlimited NAIFSPK consent, and
// pre-release.yml's network job runs on a clean runner — setup-go's cache
// covers Go modules, not astrogo's data directory. Every scheduled run would
// pull 1.58 GB from naif.jpl.nasa.gov rather than 114 MB. These suites already
// lean on JPL, USNO, NASA and ESO, and courtesy to other people's servers is
// part of the contract here.
//
// Nothing about the test's purpose is lost by the choice: the bound exists to
// catch a kernel, segment or time-scale fault, and it clears the kernel
// difference by sixty times either way. What is lost is attribution, and that
// is recovered by writing the number down instead of paying for it every week.
func TestJPLStateAgainstHorizons(t *testing.T) {
	position := metrology.NewSuite("ephemeris.jpl.horizons.position", horizonsStateReference(),
		metrology.MustContract(1e-9, "AU",
			"sits sixty times above the DE440/DE441 kernel difference (1.6e-11 AU at the Moon) "+
				"and seven times below one second of time-scale error (6.7e-09 AU), so it "+
				"discriminates a kernel, segment or time-scale fault from the ephemeris gap "+
				"the two sides genuinely have",
			"Moon geocentric speed ~1 km/s; DE440 vs DE441 measured at 1.6e-11 AU (2.42 m)"))

	velocity := metrology.NewSuite("ephemeris.jpl.horizons.velocity", horizonsStateReference(),
		metrology.MustContract(1e-10, "AU/day",
			"0.17 mm/s, far below any physical disagreement between two JPL integrations and "+
				"far above the Chebyshev round-off actually measured; the velocity is the "+
				"derivative of the same polynomials, so it fails to the same causes",
			"same reasoning as the position bound on this comparison, kernel gap included"))

	if !testutil.Reachable(horizonsHost) {
		metrology.NotVerified(t, "JPL Horizons is unreachable", position, velocity)
	}

	p, err := jpl.NewProvider(context.Background(), core.Planets, "de440")
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	defer func() { _ = p.Close() }()

	cases := loadCases(t)
	if len(cases) == 0 {
		metrology.NotVerified(t, "JPL Horizons is not answering with API data", position, velocity)
	}

	byName := make(map[string]horizonsCase, len(cases))
	for _, c := range horizonsCases() {
		byName[c.name] = c
	}

	for _, sv := range cases {
		hc, ok := byName[sv.Body]
		if !ok {
			t.Errorf("Horizons returned an unrequested body %q", sv.Body)

			continue
		}

		tm := time.FromJD(2451545.0+sv.ET/86400.0, time.TDB)

		state, err := p.State(hc.id, tm)
		if err != nil {
			t.Errorf("%s: State() failed: %v", sv.Body, err)

			continue
		}

		diffPos := state.Pos.Sub(vector.Vec3{X: sv.Pos[0], Y: sv.Pos[1], Z: sv.Pos[2]}).Norm()
		diffVel := state.Vel.Sub(vector.Vec3{X: sv.Vel[0], Y: sv.Vel[1], Z: sv.Vel[2]}).Norm()

		context := fmt.Sprintf("J2000.0 TDB, |dPos| = %.3f m", diffPos*kmPerAU*1e3)

		position.Add(metrology.Sample{Error: diffPos, Label: sv.Body, Context: context})
		velocity.Add(metrology.Sample{Error: diffVel, Label: sv.Body, Context: context})
	}

	position.Report(t)
	velocity.Report(t)
}
