package coord_test

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// barnardDR3 is Barnard's star as Gaia DR3 publishes it, at J2016.0, where
// its 10.4″/yr carries it some 166″ from where it was at J2000.
func barnardDR3(epoch time.Time) coord.ICRS {
	return coord.NewICRSWithKinematics(
		angle.Deg(269.44850252543836), angle.Deg(4.739420051112412),
		angle.Arcsec(-0.80188), angle.Arcsec(10.36211), angle.Arcsec(0.54670), unit.KmPerSec(-110.4),
		epoch,
	)
}

// j2016 is the Gaia DR3 reference epoch, J2016.0 TDB.
func j2016() time.Time { return time.FromJD(2457389.0, time.TDB) }

// TestAStarIsPlacedFromItsOwnEpoch: a star given at J2016.0, as Gaia DR3
// gives it, is observed where the same star moved to J2000.0 first is, and
// converts to FK5 as it does; the same row read as J2000.0 is placed 16
// years of proper motion off. FK4 and FK5 conversions see it at J2000 too.
func TestAStarIsPlacedFromItsOwnEpoch(t *testing.T) {
	t.Parallel()

	row := barnardDR3(j2016())
	if !row.Epoch().Equal(j2016()) {
		t.Fatalf("Epoch() = %v, want J2016.0", row.Epoch())
	}

	atJ2000, err := coord.PropagateEpoch(row, time.J2000())
	if err != nil {
		t.Fatal(err)
	}

	ctx, err := coord.NewContext(time.Date(2026, 6, 1, 4, 0, 0, 0, time.LocationUTC),
		coord.MustGeodetic(angle.LatDeg(-24.6272), angle.LonDeg(-70.4045), unit.Meters(2635)), atmosphere.Refraction{})
	if err != nil {
		t.Fatal(err)
	}

	observe := func(c coord.ICRS) coord.AltAz {
		t.Helper()

		aa, err := ctx.ICRSToAltAz(c)
		if err != nil {
			t.Fatal(err)
		}

		return aa
	}

	got, want, stale := observe(row), observe(atJ2000), observe(barnardDR3(time.J2000()))

	if d := angularDistance(got, want).Arcseconds(); d > 1e-6 {
		t.Errorf("a J2016.0 row lands %.2e″ from the same star moved to J2000 first", d)
	}

	if d := angularDistance(stale, want).Arcseconds(); d < 150 {
		t.Errorf("the row read as J2000 lands %.1f″ away; 16 years of 10.4″/yr should put it over 150″ off", d)
	}

	gotFK5, wantFK5 := coord.ICRSToFK5(row, coord.J2000Epoch), coord.ICRSToFK5(atJ2000, coord.J2000Epoch)
	if d := coord.Separation(coord.NewICRS(gotFK5.RA(), gotFK5.Dec()), coord.NewICRS(wantFK5.RA(), wantFK5.Dec())).Arcseconds(); d > 1e-6 {
		t.Errorf("ICRSToFK5 of the J2016.0 row is %.2e″ from that of the star at J2000", d)
	}

	gotFK4, wantFK4 := coord.ICRSToFK4(row, coord.B1950), coord.ICRSToFK4(atJ2000, coord.B1950)
	if d := coord.Separation(coord.NewICRS(gotFK4.RA(), gotFK4.Dec()), coord.NewICRS(wantFK4.RA(), wantFK4.Dec())).Arcseconds(); d > 1e-6 {
		t.Errorf("ICRSToFK4 of the J2016.0 row is %.2e″ from that of the star at J2000", d)
	}
}

// TestPropagateEpochMovesFromTheStarsEpoch: PropagateEpoch starts from the
// star's own epoch and the result holds at the target epoch, so moving there
// and back returns the row.
func TestPropagateEpochMovesFromTheStarsEpoch(t *testing.T) {
	t.Parallel()

	row := barnardDR3(j2016())

	there, err := coord.PropagateEpoch(row, time.J2000())
	if err != nil {
		t.Fatal(err)
	}

	if !there.Epoch().Equal(time.J2000()) {
		t.Errorf("result Epoch() = %v, want J2000", there.Epoch())
	}

	back, err := coord.PropagateEpoch(there, j2016())
	if err != nil {
		t.Fatal(err)
	}

	if d := coord.Separation(row, back).Arcseconds(); d > 1e-6 {
		t.Errorf("J2016 → J2000 → J2016 is %.2e″ from the row", d)
	}

	if d := coord.Separation(row, there).Arcseconds(); d < 150 {
		t.Errorf("the star moved %.1f″ over 16 years, want over 150″", d)
	}
}

// TestAPositionWithoutKinematicsHasNoEpoch: a bare direction holds at every
// instant, so its Epoch is zero and ICRSFromUnitVector gives one.
func TestAPositionWithoutKinematicsHasNoEpoch(t *testing.T) {
	t.Parallel()

	if e := coord.NewICRS(angle.Deg(10), angle.Deg(20)).Epoch(); !e.IsZero() {
		t.Errorf("NewICRS(...).Epoch() = %v, want zero", e)
	}

	c := coord.ICRSFromUnitVector(coord.NewICRS(angle.Deg(10), angle.Deg(20)).ToUnitVector())
	if !c.Epoch().IsZero() || c.PmRA() != 0 || c.Dist() != 0 {
		t.Errorf("ICRSFromUnitVector gave %v with epoch %v; want a bare direction", c, c.Epoch())
	}
}

// angularDistance is the separation of two horizontal directions.
func angularDistance(a, b coord.AltAz) angle.Angle {
	return coord.Separation(coord.NewICRS(a.Az(), a.Alt()), coord.NewICRS(b.Az(), b.Alt()))
}
