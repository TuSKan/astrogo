package plan_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// FromCatalog refuses a fixed target whose coordinate is not finite (#1080).
//
// The catalog parsers read such a coordinate as absent since #1080; this is
// the backstop for a target built any other way. Before it, a NaN right
// ascension built a target that was "not observable" to every constraint but
// Airmass, never rose, and dropped out of a ranking and a schedule, all
// without an error.
func TestFromCatalogRefusesANonFiniteCoordinate(t *testing.T) {
	t.Parallel()

	nan, inf := math.NaN(), math.Inf(1)

	for _, c := range []struct {
		name    string
		ra, dec float64
		kind    resolve.Kind
	}{
		{"NaN RA star", nan, -30, resolve.KindStar},
		{"infinite Dec star", 83.8, inf, resolve.KindStar},
		{"NaN Dec galaxy", 10.68, nan, resolve.KindGalaxy},
	} {
		_, err := plan.FromCatalog(resolve.Target{
			Name: c.name, Kind: c.kind, HasCoord: true,
			Coord: coord.NewICRS(angle.Deg(c.ra), angle.Deg(c.dec)),
		}, nil)
		if !errors.Is(err, plan.ErrNoCoordinates) {
			t.Errorf("%s: got %v, want ErrNoCoordinates", c.name, err)
		}
	}
}

// A fixed target's non-finite proper motion, parallax, radial velocity and
// magnitude are read as absent, and the target keeps its real position.
//
// A NaN proper motion was the worst of them: at Gaia's J2016 epoch it made a
// star with a good position NaN on every other date, so it never rose.
func TestFromCatalogReadsNonFiniteExtrasAsAbsent(t *testing.T) {
	t.Parallel()

	nan := math.NaN()
	gaiaEpoch := time.FromJD(2457389.0, time.TT) // J2016.0
	night := time.FromJD(2461000.5, time.UTC).Add(unit.Hours(3))

	base := resolve.Target{
		Name: "Rigel", Kind: resolve.KindStar, HasCoord: true,
		Coord: coord.NewICRS(angle.Deg(78.634), angle.Deg(-8.2016)),
		Epoch: gaiaEpoch,
	}

	clean, err := plan.FromCatalog(base, nil)
	if err != nil {
		t.Fatalf("FromCatalog(clean): %v", err)
	}

	want, err := clean.Position(night)
	if err != nil {
		t.Fatalf("Position(clean): %v", err)
	}

	broken := base
	broken.PmRA, broken.PmDec, broken.Parallax = angle.Rad(nan), angle.Rad(nan), angle.Rad(nan)
	broken.RadialVelocity, broken.HasRadialVelocity = unit.Velocity(nan), true
	broken.VMag, broken.HasVMag = nan, true

	obj, err := plan.FromCatalog(broken, nil)
	if err != nil {
		t.Fatalf("FromCatalog(broken): %v", err)
	}

	got, err := obj.Position(night)
	if err != nil {
		t.Fatalf("Position(broken): %v", err)
	}

	if got.RA() != want.RA() || got.Dec() != want.Dec() {
		t.Errorf("with NaN kinematics the star is at %v, %v; with none it is at %v, %v",
			got.RA().Degrees(), got.Dec().Degrees(), want.RA().Degrees(), want.Dec().Degrees())
	}

	// The control first, so the two checks after it cannot pass by the star
	// not carrying a magnitude or a radial velocity at all.
	measured := base
	measured.VMag, measured.HasVMag = 0.13, true
	measured.RadialVelocity, measured.HasRadialVelocity = unit.KmPerSec(17.8), true

	withReal, err := plan.FromCatalog(measured, nil)
	if err != nil {
		t.Fatalf("FromCatalog(real): %v", err)
	}

	m, okM := withReal.(plan.StaticMagnitude)
	rv, okRV := withReal.(plan.MeasuredRadialVelocity)

	if !okM || !okRV {
		t.Fatalf("a %T reports neither a magnitude nor a radial velocity; this test cannot "+
			"tell them absent", withReal)
	}

	if _, has := m.StaticMagnitude(); !has {
		t.Fatal("a real magnitude was not kept")
	}

	if _, has := rv.MeasuredRadialVelocity(); !has {
		t.Fatal("a real radial velocity was not kept")
	}

	// The same type as the control, so both interfaces hold for it too.
	brokenM, okM := obj.(plan.StaticMagnitude)
	brokenRV, okRV := obj.(plan.MeasuredRadialVelocity)

	if !okM || !okRV {
		t.Fatalf("the broken target is a %T, not the control's %T", obj, withReal)
	}

	if v, has := brokenM.StaticMagnitude(); has {
		t.Errorf("a NaN magnitude was kept as %v", v)
	}

	if v, has := brokenRV.MeasuredRadialVelocity(); has {
		t.Errorf("a NaN radial velocity was kept as %v", v)
	}
}
