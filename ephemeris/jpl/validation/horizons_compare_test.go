//go:build network

package jpl_test

import (
	"context"
	"errors"
	"fmt"
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
		sv, err := fetchVector(c.naifID, c.name, start, stop)
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
// Horizons serves DE441 and the local provider reads DE440, so both sides are
// JPL ephemerides. That makes this a consistency check on astrogo's SPK
// evaluation and time-scale handling rather than independent validation of
// the ephemeris, and the shared ancestry is recorded so the generated table
// says so.
func horizonsStateReference() metrology.Reference {
	return metrology.Reference{
		Kind:           metrology.KindHorizons,
		Name:           "JPL Horizons",
		Version:        "VECTORS, geocentric, ICRF",
		Source:         "https://ssd.jpl.nasa.gov/api/horizons.api",
		Dataset:        "DE441 (Horizons) against DE440 (local)",
		SharedAncestor: "JPL DE",
	}
}

// TestJPLStateAgainstHorizons compares astrogo's DE440 evaluation against
// Horizons' own state vectors for the same instant.
//
// # Why the tolerances moved by five orders of magnitude
//
// They were 1e-7 AU and 1e-8 AU/day, with no stated reason. Measured, the two
// agree to 5.5e-14 AU for the Sun and Mars and 4.5e-12 AU for the Moon — the
// old bound was about two million times the largest real residual, which is a
// bound that cannot fail for the reason it exists. docs/VALIDATION.md
// published those two numbers as the achieved accuracy of astrogo's
// ephemerides, which they never were.
//
// The bound below is derived from the smallest fault worth catching rather
// than from what was measured. Both sides evaluate the same JPL integration —
// DE441 and DE440 share it over this span — so a real disagreement is not an
// ephemeris difference but a fault in kernel selection, segment choice or
// time scale. The cheapest such fault to make is a one-second time-scale
// error, which this repository had until recently in its leap-second parsing;
// one second moves the Moon about a kilometre, or 6.7e-9 AU. A bound of
// 1e-9 AU sits below that and two hundred times above the worst residual
// actually observed, so it catches the fault class without tracking the noise.
func TestJPLStateAgainstHorizons(t *testing.T) {
	position := metrology.NewSuite("ephemeris.jpl.horizons.position", horizonsStateReference(),
		metrology.MustContract(1e-9, "AU",
			"one second of time-scale error moves the Moon about a kilometre (6.7e-09 AU), and "+
				"both sides evaluate the same JPL integration, so anything above this is a kernel, "+
				"segment or time-scale fault rather than a difference between ephemerides",
			"Moon geocentric speed ~1 km/s; DE440 and DE441 share their integration over this span"))

	velocity := metrology.NewSuite("ephemeris.jpl.horizons.velocity", horizonsStateReference(),
		metrology.MustContract(1e-10, "AU/day",
			"0.17 mm/s, far below any physical disagreement between two evaluations of one "+
				"integration and far above the Chebyshev round-off actually measured; the velocity "+
				"is the derivative of the same polynomials, so it fails to the same causes",
			"same reasoning as the position bound on this comparison"))

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
