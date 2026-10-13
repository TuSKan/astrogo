package posmatch

import (
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

	// Not an exact zero: NewICRSWithKinematics and NewICRS build the vector by
	// different arithmetic, and on arm64 fused multiply-add leaves 2e-13″
	// between them. A microarcsecond is far below any catalog's precision and
	// far above that rounding.
	if sep := coord.Separation(c, barnard.Coord).Arcseconds(); sep > 1e-6 {
		t.Errorf("Coord moved the position by %g″; it should only attach the motion", sep)
	}

	still := resolve.Target{Coord: barnard.Coord, HasCoord: true}
	if got := Coord(still); got != still.Coord {
		t.Errorf("Coord of a Target with no motion is %v, want its Coord unchanged", got)
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
