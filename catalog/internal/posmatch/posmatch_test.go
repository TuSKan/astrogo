package posmatch

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// barnard is Barnard's star as catalog/simbad builds it: RA and Dec in Coord,
// the motion in the Target's own fields. The row is SIMBAD's, read live on
// 2026-10-08 (#627).
var barnard = resolve.Target{
	ID: "NAME Barnard's star", Coord: coord.NewICRS(angle.Deg(269.4520769586), angle.Deg(4.6933649666)),
	HasCoord: true, Epoch: time.J2000(),
	PmRA: angle.Arcsec(-0.8015510), PmDec: angle.Arcsec(10.3623940),
	Parallax: angle.Arcsec(0.5469759), RadialVelocity: unit.KmPerSec(-110.11),
}

// TestCoordCarriesTheTargetsOwnMotion is the property the whole package
// exists for: the motion a provider leaves in the Target's fields ends up in
// the position that gets propagated, and a Target with none comes back as
// it was.
func TestCoordCarriesTheTargetsOwnMotion(t *testing.T) {
	t.Parallel()

	c := Coord(barnard)
	if c.PmRA() != barnard.PmRA || c.PmDec() != barnard.PmDec || c.Parallax() != barnard.Parallax || c.RV() != barnard.RadialVelocity {
		t.Errorf("Coord dropped the motion: pm (%v, %v) plx %v rv %v", c.PmRA(), c.PmDec(), c.Parallax(), c.RV())
	}

	// Not an exact zero: the two positions hold the same RA and Dec, but
	// coord.Separation of a point with itself is atan2(|a×a|, a·a), and on
	// arm64 fused multiply-add turns each x·y − y·x of the cross product into
	// the rounding error of one product — 2e-13″, measured on macOS CI. A
	// microarcsecond is far below any catalog's precision and far above that.
	if sep := coord.Separation(c, barnard.Coord).Arcseconds(); sep > 1e-6 {
		t.Errorf("Coord moved the position by %g″; it should only attach the motion", sep)
	}

	still := resolve.Target{Coord: barnard.Coord, HasCoord: true}
	if got := Coord(still); got != still.Coord {
		t.Errorf("Coord of a Target with no motion is %v, want its Coord unchanged", got)
	}
}

// TestCoordCarriesEachKinematicFieldOnItsOwn: Coord's shortcut returns the
// bare position only when all four fields are zero. A Target carrying just
// one of them must still get it attached. Dropping any one term from the
// shortcut's condition would hand that Target back without the value its
// provider read.
func TestCoordCarriesEachKinematicFieldOnItsOwn(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name string
		set  func(*resolve.Target)
		got  func(coord.ICRS) bool
	}{
		{"proper motion in RA", func(tg *resolve.Target) { tg.PmRA = barnard.PmRA }, func(c coord.ICRS) bool { return c.PmRA() == barnard.PmRA }},
		{"proper motion in Dec", func(tg *resolve.Target) { tg.PmDec = barnard.PmDec }, func(c coord.ICRS) bool { return c.PmDec() == barnard.PmDec }},
		{"parallax", func(tg *resolve.Target) { tg.Parallax = barnard.Parallax }, func(c coord.ICRS) bool { return c.Parallax() == barnard.Parallax }},
		{"radial velocity", func(tg *resolve.Target) { tg.RadialVelocity = barnard.RadialVelocity }, func(c coord.ICRS) bool { return c.RV() == barnard.RadialVelocity }},
	} {
		tgt := resolve.Target{Coord: barnard.Coord, HasCoord: true, Epoch: time.J2000()}
		c.set(&tgt)

		if !c.got(Coord(tgt)) {
			t.Errorf("%s alone: Coord dropped it", c.name)
		}
	}
}

// TestSeparationReportsAPositionItCannotMove: a position that cannot be
// propagated is an error, never a separation. Answering 0 instead would put
// that target within any threshold of everything it is compared with.
//
// The position here cannot be moved because of its parallax. 1e300″ puts
// the star at a distance that underflows to zero, and SOFA's Pmsafe refuses
// the space motion with status -1. It is the one way found to make
// coord.PropagateEpoch fail: every parallax up to 1e12″ propagates, and a
// NaN anywhere propagates to NaN with status 0. A catalog parser would pass
// 1e300 on, since it is finite. Each of Separation's four propagations is
// made to fail on its own: the star that cannot be moved is given at the Gaia
// epoch, so taking it to J2000, or to the other side's epoch, means moving it.
func TestSeparationReportsAPositionItCannotMove(t *testing.T) {
	t.Parallel()

	gaiaEpoch := time.FromJD(time.J2000().JD()+16*365.25, time.TT)
	unmovable := coord.NewICRSWithKinematics(angle.Deg(150), angle.Deg(20),
		angle.Arcsec(0.1), angle.Arcsec(0.1), angle.Arcsec(1e300), unit.KmPerSec(0))
	moving := Coord(barnard)
	still := coord.NewICRS(angle.Deg(150), angle.Deg(20))

	for _, c := range []struct {
		name   string
		a      coord.ICRS
		aEpoch time.Time
		b      coord.ICRS
		bEpoch time.Time
	}{
		{"both moving, the first cannot be", unmovable, gaiaEpoch, moving, time.J2000()},
		{"both moving, the second cannot be", moving, time.J2000(), unmovable, gaiaEpoch},
		{"only the first moving", unmovable, gaiaEpoch, still, time.J2000()},
		{"only the second moving", still, time.J2000(), unmovable, gaiaEpoch},
	} {
		sep, err := Separation(c.a, c.aEpoch, c.b, c.bEpoch)
		if !errors.Is(err, coord.ErrPropagationFailed) {
			t.Errorf("%s: Separation = %v, %v; want coord.ErrPropagationFailed", c.name, sep, err)
		}
	}
}

// TestSeparationMovesTheStarThatMoves: Barnard's star is about 10.4″/yr, so
// the same row given at J2000 and a decade later is 104″ from itself unless
// the motion is applied. With it applied the two are the same star.
func TestSeparationMovesTheStarThatMoves(t *testing.T) {
	t.Parallel()

	c := Coord(barnard)
	laterEpoch := time.FromJD(time.J2000().JD()+3652.5, time.TT)

	later, err := coord.PropagateEpoch(c, barnard.Epoch, laterEpoch)
	if err != nil {
		t.Fatal(err)
	}

	if unmoved := coord.Separation(barnard.Coord, later).Arcseconds(); unmoved < 100 {
		t.Fatalf("fixture: a decade of motion is only %.1f″", unmoved)
	}

	// Both moving: compared at J2000.
	sep, err := Separation(c, barnard.Epoch, later, laterEpoch)
	if err != nil {
		t.Fatal(err)
	}

	if s := sep.Arcseconds(); s > 0.01 {
		t.Errorf("both moving: %.4f″ apart, want the same star", s)
	}

	// One moving: the moving one is taken to the other's epoch, whichever
	// side it is on.
	still := coord.NewICRS(later.RA(), later.Dec())

	for _, c2 := range []struct {
		name string
		sep  func() (angle.Angle, error)
	}{
		{"moving first", func() (angle.Angle, error) { return Separation(c, barnard.Epoch, still, laterEpoch) }},
		{"moving second", func() (angle.Angle, error) { return Separation(still, laterEpoch, c, barnard.Epoch) }},
	} {
		sep, err := c2.sep()
		if err != nil {
			t.Fatalf("%s: %v", c2.name, err)
		}

		if s := sep.Arcseconds(); s > 0.01 {
			t.Errorf("%s: %.4f″ apart, want the same star", c2.name, s)
		}
	}
}
