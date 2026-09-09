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
// Both sides now read DE441 — Horizons reports it as the source and the local
// provider loads de441_part-2 — which is deliberate: the ephemeris is not
// under test, so holding it identical removes it as a variable. That also
// makes this unambiguously a consistency check on astrogo's SPK evaluation
// and time-scale handling rather than validation of the ephemeris itself, and
// the shared ancestry is recorded so the generated table says so instead of
// leaving a reader to work it out from the version string.
func horizonsStateReference() metrology.Reference {
	return metrology.Reference{
		Kind:           metrology.KindHorizons,
		Name:           "JPL Horizons",
		Version:        "VECTORS, geocentric, ICRF",
		Source:         "https://ssd.jpl.nasa.gov/api/horizons.api",
		Dataset:        "DE441 on both sides (de441_part-2 locally)",
		SharedAncestor: "JPL DE441 — the same ephemeris, by design",
	}
}

// TestJPLStateAgainstHorizons compares astrogo's DE441 evaluation against
// Horizons' own state vectors for the same instant.
//
// # Why the tolerances moved by five orders of magnitude
//
// They were 1e-7 AU and 1e-8 AU/day, with no stated reason — about a hundred
// thousand times the largest real residual, which is a bound that cannot fail
// for the reason it exists. docs/VALIDATION.md published the measured figures
// as the achieved accuracy of astrogo's ephemerides, which they never were.
//
// The bound below is derived from the smallest fault worth catching rather
// than from what was measured. Both sides evaluate the same JPL integration,
// so a real disagreement is not an ephemeris difference but a fault in kernel
// selection, segment choice or time scale. The cheapest such fault to make is
// a one-second time-scale error, which this repository had until recently in
// its leap-second parsing; one second moves the Moon about a kilometre, or
// 6.7e-9 AU. A bound of 1e-9 AU sits below that and far above the worst
// residual actually observed, so it catches the fault class without tracking
// the noise.
//
// # Why de441_part-2 and not de440
//
// Because "both sides evaluate the same JPL integration" has to be true for
// that reasoning to hold, and it was not. Horizons reports
// {source: DE441} for these queries while this test read de440, and the two
// do *not* agree over this span — measured directly, geocentric positions
// differ by 2.94 cm for the Sun, Mars and Jupiter and by 2.42 m for the Moon.
//
// That was not a rounding detail, it was the residual. Switching to the
// ephemeris Horizons actually serves:
//
//	              position max            velocity max
//	de440         4.52e-12 AU (0.676 m)   1.006e-12 AU/day   worst body: Moon
//	de441_part-2  9.80e-13 AU (0.147 m)   1.405e-14 AU/day   worst body: Mars
//
// The position agrees 4.6 times more closely and the velocity 72 times, and
// the Moon stops being the worst body — its residual *was* the difference
// between the two kernels, attributed for as long as this test existed to
// astrogo (#255).
//
// It costs about five seconds: de441_part-2 is 1.65 GB against de440's 119 MB
// and takes 5.2 s to open rather than 0.38 s, since opening indexes every
// segment. Queries are no slower once open. The other de440 users in this
// package keep it deliberately — kepler_pluto, scaleinvariance and the two
// sofa_compare suites measure residuals from kilometres to 960,000 km, five to
// seven orders of magnitude above a 3 cm kernel difference, so paying five
// seconds each to remove it would buy nothing.
func TestJPLStateAgainstHorizons(t *testing.T) {
	position := metrology.NewSuite("ephemeris.jpl.horizons.position", horizonsStateReference(),
		metrology.MustContract(1e-9, "AU",
			"one second of time-scale error moves the Moon about a kilometre (6.7e-09 AU), and "+
				"both sides now read DE441, so anything above this is a kernel, segment or "+
				"time-scale fault rather than a difference between ephemerides",
			"Moon geocentric speed ~1 km/s; both sides read DE441, measured max 9.8e-13 AU"))

	velocity := metrology.NewSuite("ephemeris.jpl.horizons.velocity", horizonsStateReference(),
		metrology.MustContract(1e-10, "AU/day",
			"0.17 mm/s, far below any physical disagreement between two evaluations of one "+
				"integration and far above the Chebyshev round-off actually measured; the velocity "+
				"is the derivative of the same polynomials, so it fails to the same causes",
			"same reasoning as the position bound on this comparison"))

	if !testutil.Reachable(horizonsHost) {
		metrology.NotVerified(t, "JPL Horizons is unreachable", position, velocity)
	}

	p, err := jpl.NewProvider(context.Background(), core.Planets, "de441_part-2")
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
