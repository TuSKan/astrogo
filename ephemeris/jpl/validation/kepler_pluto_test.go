//go:build validation

package jpl_test

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/ephemeris/kepler"
	"github.com/TuSKan/astrogo/internal/metrology"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// plutoReference records what the Kepler propagation is measured against,
// and why the comparison is not an independent check of Pluto's orbit.
//
// The Standish elements are themselves a least-squares fit to a JPL
// development ephemeris. Comparing them against DE440 therefore measures what
// the two-body approximation loses relative to the integration it was fitted
// from — which is the useful question for a caller choosing between them, and
// is not the same as validating either against an outside authority.
func plutoReference() metrology.Reference {
	return metrology.Reference{
		Kind:           metrology.KindSPICE,
		Name:           "JPL DE440",
		Version:        "de440.bsp",
		Source:         "https://naif.jpl.nasa.gov/pub/naif/generic_kernels/spk/planets/",
		Dataset:        "two-body propagation of Standish mean elements against the integration",
		SharedAncestor: "JPL — the elements are a fit to a JPL development ephemeris",
	}
}

// plutoContract bounds what [kepler.PlutoElements] is worth.
//
// # Why there is no published figure to cite
//
// There used to be. The elements come from Standish's "Keplerian Elements for
// Approximate Positions of the Major Planets", whose Table 1 this repository
// cites as valid 1800-2050 — and JPL has since removed Pluto from that
// document entirely: "The former planet Pluto has also been removed." The
// current edition states accuracies for Mercury through Neptune and nothing
// for Pluto, so the obvious source for this bound no longer exists.
//
// # What the bound is instead
//
// The same construction the Galilean suite uses, and for the same reason: it
// is not an accuracy claim, it is a bound sitting between two measured errors
// so that it fails for a structural fault and passes while the physics is
// merely absent.
//
//   - Below it, two-body motion is wrong here by construction. Pluto is in a
//     3:2 resonance with Neptune and this propagation models none of it. That
//     costs at most 0.1375 AU over 1800-2050, the elements' own validity
//     window — and it is worth noting the error is 0.037 AU over 2000-2050
//     and grows to 0.1375 AU only at the 1800 edge.
//   - Above it, a structural fault is enormous. Omitting the ecliptic-to-
//     equatorial rotation — the realistic way to get this wrong, since the
//     elements are published in one frame and the result is returned in
//     another — displaces Pluto by at least 3.54 AU. See
//     TestPlutoElementsCarryTheObliquityRotation, which measures it.
//
// The "at least" matters and cost a revision to get right. A first version of
// this file sampled that displacement at four epochs and read the smallest as
// 7.2 AU; sampling yearly found 3.54, because the displacement depends on
// where Pluto is relative to the rotation axis and a coarse grid simply misses
// the shallow years. The bound was set from the wrong anchor until the guard
// test disagreed with it.
//
// 0.7 AU is the geometric mean of the two: 5.1x above the worst approximation
// error and 5.1x below the smallest structural one. Placing it in the middle
// rather than just above the measured drift is deliberate — a bound pinned to
// a measurement cannot fail for the reason it exists.
func plutoContract() metrology.Contract {
	return metrology.MustContract(0.7, "AU",
		"not an accuracy claim. It sits between two measured errors, at the geometric mean of "+
			"both: two-body motion ignores the 3:2 Neptune resonance and costs at most 0.1375 AU "+
			"over 1800-2050, while omitting the ecliptic-to-equatorial rotation displaces Pluto "+
			"by at least 3.54 AU. A bound 5.1x above the first and 5.1x below the second fails "+
			"when the frame or the elements are wrong and passes while the perturbations are "+
			"merely unmodelled, which is the only distinction this suite can honestly make. "+
			"JPL no longer publishes an accuracy figure for Pluto — it was removed from the "+
			"Standish document these elements come from",
		"both anchors measured: this suite for the approximation error, "+
			"TestPlutoElementsCarryTheObliquityRotation for the structural one")
}

// TestKeplerPlutoAgainstDE440 measures what the offline Pluto is worth.
//
// # Why it matters more than a validation row usually does
//
// This is the one body in eph.Default() that is not SOFA. SOFA has no
// analytical Pluto, so kepler.PlutoElements is what answers — and until this
// suite existed, the size of that approximation was stated nowhere. The
// package doc calls it "an approximate offline position, not a claim of
// high-precision Pluto ephemeris", which is honest and is not a number.
//
// It is also the worst body in the default provider by a wide margin: the
// planets sit at arcseconds to arcminutes against DE440, and this sits at
// arcminutes to a degree. A caller who reads "offline ephemeris" and gets the
// same quality for Pluto as for Mars would be wrong by two orders of
// magnitude, and now the table says so.
func TestKeplerPlutoAgainstDE440(t *testing.T) {
	suite := metrology.NewSuite("ephemeris.kepler.pluto", plutoReference(), plutoContract())

	p, err := jpl.NewProvider(context.Background(), core.Planets, "de440")
	if err != nil {
		metrology.NotVerified(t, "the JPL provider could not be built: "+err.Error(), suite)
	}

	defer func() { _ = p.Close() }()

	// The elements' own validity window, not the kernel's. Sampling outside
	// 1800-2050 would measure a fit where it never claimed to hold.
	const (
		firstYear = 1800
		lastYear  = 2050
	)

	base := kepler.New()

	for y := firstYear; y <= lastYear; y++ {
		for _, mo := range []time.Month{time.January, time.April, time.July, time.October} {
			at := time.Date(y, mo, 1, 0, 0, 0, 0, time.LocationUTC)

			want, werr := p.State(core.Pluto, at)
			if werr != nil {
				t.Fatalf("DE440 State(Pluto) at %s: %v", at.Format(isoDate), werr)
			}

			got, gerr := base.State(core.Pluto, at)
			if gerr != nil {
				t.Fatalf("Kepler State(Pluto) at %s: %v", at.Format(isoDate), gerr)
			}

			diff := got.Pos.Sub(want.Pos).Norm()

			suite.Add(metrology.Sample{
				Error: diff,
				Label: "Pluto",
				Context: fmt.Sprintf("%s, |r| = %.1f AU, |DE440 - Kepler| = %.4f AU",
					at.Format(isoDate), want.Pos.Norm(), diff),
			})
		}
	}

	suite.Report(t)
}

// TestPlutoElementsCarryTheObliquityRotation measures the upper anchor
// [plutoContract] is built on, and guards the rotation itself.
//
// The elements are published in the J2000 mean ecliptic frame and StateAt
// returns ICRS equatorial, so exactly one obliquity rotation stands between
// them. Omitting it is the realistic way to get this wrong: nothing about the
// numbers looks different, the orbit stays the right size and shape, and Pluto
// simply ends up somewhere else.
//
// The test undoes the rotation by hand and measures how far that moves the
// body. If the answer ever collapses toward zero, the rotation is no longer
// being applied — and the contract that rests on this number would be
// bounding nothing.
func TestPlutoElementsCarryTheObliquityRotation(t *testing.T) {
	// IAU 2006 mean obliquity at J2000.0.
	const obliquityDeg = 23.4392911

	obliquity := obliquityDeg * math.Pi / 180
	sinE, cosE := math.Sin(obliquity), math.Cos(obliquity)

	smallest := math.Inf(1)

	for y := 1800; y <= 2050; y++ {
		at := time.Date(y, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

		pos, _, err := kepler.PlutoElements.StateAt(at)
		if err != nil {
			t.Fatalf("StateAt(%d): %v", y, err)
		}

		// Rotate back about X, which is what a missing ecliptic-to-
		// equatorial step would have left.
		unrotated := vector.V3(pos.X, pos.Y*cosE+pos.Z*sinE, -pos.Y*sinE+pos.Z*cosE)

		smallest = math.Min(smallest, pos.Sub(unrotated).Norm())
	}

	t.Logf("a missing obliquity rotation displaces Pluto by at least %.2f AU over 1800-2050",
		smallest)

	// The contract cites 3.54 AU as its upper anchor. This asserts the anchor
	// rather than trusting the comment: a range, not an equality, because the
	// figure depends on where Pluto happens to be and the point is only that
	// it stays an order of magnitude above the approximation error.
	//
	// Sample yearly. At a ten-year step this minimum reads 3.59, and at four
	// epochs it reads 7.2 — the displacement is shallow in the years when
	// Pluto sits near the rotation axis, and a coarse grid steps over them.
	// The first version of this file set the contract from that 7.2.
	if smallest < 2.5 || smallest > 6 {
		t.Errorf("smallest structural displacement is %.2f AU, expected about 3.54 — "+
			"plutoContract's upper anchor no longer holds", smallest)
	}
}
