//go:build network

package jpl_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/ephemeris/jpl/spk"
	"github.com/TuSKan/astrogo/internal/metrology"
	"github.com/TuSKan/astrogo/internal/testutil"
	atime "github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// smallBody is one asteroid to validate, with the reason it is in the set.
//
// Horizons matches small bodies by number or by its own designation syntax,
// not by name: "433" and "433;" resolve to Eros while "Eros" does not, and
// "DES=433;" resolves to a different object entirely (248370). The semicolon
// form is used throughout because it is the one that means "small body" to
// Horizons rather than "look this up in the planetary table first".
type smallBody struct {
	id          int
	designation string
	name        string
	why         string
}

func smallBodies() []smallBody {
	return []smallBody{
		{
			id: 433, designation: "433;", name: "Eros",
			why: "near-Earth, and the only small body this repository had any position test for",
		},
		{
			id: 3200, designation: "3200;", name: "Phaethon",
			why: "e=0.89 with perihelion inside Mercury's orbit, so its difference arrays " +
				"span a far wider range of speeds than a main-belt body's",
		},
		{
			id: 16, designation: "16;", name: "Psyche",
			why: "main belt at ~3 AU, the slow end of the range",
		},
		{
			id: 1, designation: "1;", name: "Ceres",
			why: "the largest asteroid, and one of the four that were unreachable until " +
				"small-body identifiers stopped colliding with the planets",
		},
		{
			id: 4, designation: "4;", name: "Vesta",
			why: "the collision that exposed the identifier bug: asteroid 4 landed on Mars",
		},
		{
			id: 624, designation: "624;", name: "Hektor",
			why: "Jupiter trojan at ~5.2 AU, the slowest and most distant case here",
		},
	}
}

// Ceres and Vesta are in the set above deliberately, and were not always
// loadable.
//
// A small body used to be identified by its bare number, so asteroid 4 Vesta
// and Mars were both core.ID(4) — as were Ceres and Mercury, Pallas and
// Venus, and every asteroid numbered up to 12. A provider holding a planetary
// kernel resolved the planet and dropped the asteroid as a duplicate, and
// ErrNoSmallBodyKernel then blamed the designation, which was never the
// cause: Horizons returns a perfectly good 72,704-byte Type 21 kernel for
// "4;". Small bodies now keep NAIF's own 20000000+ identifier — see
// [core.SmallBodyID] — and the two namespaces cannot overlap.
//
// Keeping the two worst-affected bodies in this suite is the point: they are
// the cases that would silently disappear again.

// smallBodySPKReference records what this comparison is against.
//
// Horizons generates the SPK and Horizons produces the reference vectors, so
// both sides descend from the same JPL small-body solution. That makes this a
// check on astrogo's Type 21 decoding and evaluation rather than independent
// validation of the orbit, and the shared ancestry is recorded so the
// generated table says so rather than leaving a reader to assume otherwise.
func smallBodySPKReference() metrology.Reference {
	return metrology.Reference{
		Kind:           metrology.KindHorizons,
		Name:           "JPL Horizons",
		Version:        "VECTORS, geocentric, ICRF",
		Source:         "https://ssd.jpl.nasa.gov/api/horizons.api",
		Dataset:        "Horizons-generated SPK (Type 21) against Horizons' own vectors",
		SharedAncestor: "JPL small-body solution",
	}
}

// TestSmallBodySPKAgainstHorizons compares astrogo's evaluation of a
// Horizons-generated small-body kernel against Horizons' own state vectors.
//
// # Why this suite exists
//
// Until now the only assertion anywhere on a small-body position was
// 0.1 AU < |r| < 5.0 AU, in ephemeris/jpl's TestSmallBodyEros. That bound
// spans two orders of magnitude and is satisfied by most of the inner solar
// system, so it cannot distinguish a correct position from one that is wrong
// by an astronomical unit.
//
// What it guards is worth more than that. Small-body kernels are SPK
// **Type 21** — Extended Modified Difference Arrays — which is a different
// decoder from the Type 2/3 Chebyshev polynomials the planetary kernels use,
// hand-rolled in this repository, and one where a decoding defect has
// previously corrupted small-body positions. TestJPLStateAgainstHorizons
// covers the Chebyshev path at 1e-9 AU; nothing covered this one.
//
// # What it can and cannot show
//
// Horizons generates the kernel and Horizons produces the reference, so a
// disagreement is not an orbit difference: it is astrogo mis-decoding or
// mis-evaluating a file whose contents both sides agree on. That is exactly
// the fault class Type 21 is exposed to, and it is not independent validation
// of the orbit itself — see [smallBodySPKReference].
func TestSmallBodySPKAgainstHorizons(t *testing.T) {
	position := metrology.NewSuite("ephemeris.jpl.smallbody.position", smallBodySPKReference(),
		metrology.MustContract(1e-11, "AU",
			"1.5 m. Both sides read the same difference arrays, so the residual is floating-point "+
				"scatter between two evaluation orders, measured at 2.2e-13 AU (33 mm) across four "+
				"bodies spanning 1 to 5 AU and eccentricity 0.08 to 0.89. This sits ~45x above that "+
				"scatter, leaving room for other epochs and platforms without tracking the noise, "+
				"and two orders of magnitude below the 1e-9 AU the Chebyshev path allows, so a "+
				"regression in this decoder is caught long before it reaches the planetary bound",
			"measured 2024-01-01 to 2024-02-10 against Horizons' own vectors; cf. "+
				"TestJPLStateAgainstHorizons for the Chebyshev path"))

	velocity := metrology.NewSuite("ephemeris.jpl.smallbody.velocity", smallBodySPKReference(),
		metrology.MustContract(1e-12, "AU/day",
			"1.7 mm/s. The velocity is read from the same difference arrays as the position and "+
				"fails to the same causes, so it is bounded the same way: ~22x the 4.6e-14 AU/day "+
				"scatter measured, and two orders below the Chebyshev path's 1e-10 AU/day",
			"same measurement as the position bound on this comparison"))

	if !testutil.Reachable(horizonsHost) {
		metrology.NotVerified(t, "JPL Horizons is unreachable", position, velocity)
	}

	// A span short enough that one Horizons SPK covers it comfortably, and
	// long enough that a decoding fault at a difference-array boundary has
	// somewhere to show up.
	const (
		startJD = 2460310.5 // 2024-01-01
		days    = 40
		stepJD  = 4

		// The kernel is asked for more than the reference series covers.
		// Requesting exactly the same span leaves the final epoch on the
		// segment's own boundary, where the first run of this suite got
		// "no coverage for target at requested epoch" for every body.
		marginDays = 5
	)

	start := atime.FromJD(startJD-marginDays, atime.TDB)
	stop := atime.FromJD(startJD+days+marginDays, atime.TDB)

	for _, body := range smallBodies() {
		t.Run(body.name, func(t *testing.T) {
			p, err := jpl.NewProvider(context.Background(), core.SmallBody, body.designation,
				jpl.WithTimeInterval(start, stop))
			if err != nil {
				// Horizons can return a syntactically valid but empty SPK
				// for a request it otherwise accepts — an external anomaly
				// this repository already detects and names rather than
				// caching a broken kernel.
				if errors.Is(err, spk.ErrHorizonsEmptyKernel) {
					t.Skipf("Horizons returned an unusable SPK for %s: %v", body.name, err)
				}

				t.Fatalf("provider for %s (%s): %v", body.name, body.designation, err)
			}

			defer func() { _ = p.Close() }()

			refs, err := fetchVectorSeries(body.designation, body.name,
				"2024-01-01 00:00 TDB", "2024-02-10 00:00", fmt.Sprintf("%dd", stepJD))
			if err != nil {
				t.Skipf("Horizons vectors for %s: %v", body.name, err)
			}

			if len(refs) == 0 {
				t.Skipf("Horizons returned no vectors for %s", body.name)
			}

			for _, ref := range refs {
				// ET is seconds past J2000.0 TDB, the same convention
				// TestJPLStateAgainstHorizons reads.
				when := atime.FromJD(2451545.0+ref.ET/86400.0, atime.TDB)

				state, serr := p.State(core.SmallBodyID(body.id), when)
				if serr != nil {
					t.Errorf("%s: State at JD %.5f: %v", body.name, when.JD(), serr)

					continue
				}

				dPos := state.Pos.Sub(vector.Vec3{X: ref.Pos[0], Y: ref.Pos[1], Z: ref.Pos[2]}).Norm()
				dVel := state.Vel.Sub(vector.Vec3{X: ref.Vel[0], Y: ref.Vel[1], Z: ref.Vel[2]}).Norm()

				ctx := fmt.Sprintf("JD %.5f TDB, |dPos| = %.3f m", when.JD(), dPos*kmPerAU*1e3)

				position.Add(metrology.Sample{Error: dPos, Label: body.name, Context: ctx})
				velocity.Add(metrology.Sample{Error: dVel, Label: body.name, Context: ctx})
			}

			t.Logf("%s (%s): %s", body.name, body.designation, body.why)
		})
	}

	if position.Len() == 0 {
		metrology.NotVerified(t, "no small body produced a comparable state", position, velocity)
	}

	position.Report(t)
	velocity.Report(t)
}
