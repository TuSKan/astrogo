package kepler

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// centerEpoch is the osculation epoch of the orbits the center tests build.
var centerEpoch = time.FromJDParts(2460676.5, 0, time.TDB) // 2025-01-01

// registered returns what p stores for el.
func registered(t *testing.T, el Elements) Elements {
	t.Helper()

	p := New()
	if err := p.Register(1_000_000, el); err != nil {
		t.Fatalf("Register: %v", err)
	}

	return p.elements[1_000_000]
}

func orbitAt(t *testing.T, a, e float64) Elements {
	t.Helper()

	el, err := NewElements(centerEpoch, unit.AU(a), e, angle.Deg(11), angle.Deg(73), angle.Deg(150), angle.Deg(40))
	if err != nil {
		t.Fatal(err)
	}

	return el
}

// TestRegisterKeepsAnAsteroidAboutTheSun: inside Jupiter's orbit the planets
// pull an asteroid least about the Sun, and Register keeps it there.
func TestRegisterKeepsAnAsteroidAboutTheSun(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		a, e float64
	}{
		{"a near-Earth asteroid, Eros's orbit", 1.458, 0.223},
		{"the main belt, Ceres's orbit", 2.77, 0.079},
	} {
		el := orbitAt(t, tc.a, tc.e)

		choice, err := chooseCenter(el)
		if err != nil {
			t.Fatal(err)
		}

		if got := registered(t, el).CentralBody().ID; got != core.Sun {
			t.Errorf("%s: fitted about %v (perturbation helio %.3g, bary %.3g), want the Sun",
				tc.name, got, choice.helio, choice.bary)
		}
	}
}

// TestRegisterRefitsAFarBodyAboutTheBarycenter: beyond the giant planets the
// Sun and every planet pull together, and a two-body fit about their
// barycenter, with all of them as its mass, leaves out least. The refit starts
// from the given orbit's state at its osculation epoch, exactly.
func TestRegisterRefitsAFarBodyAboutTheBarycenter(t *testing.T) {
	t.Parallel()

	el := orbitAt(t, 45.6, 0.155) // Makemake's orbit, roughly

	fitted := registered(t, el)

	if got := fitted.CentralBody().ID; got != core.SolarSystemBarycenter {
		t.Fatalf("fitted about %v, want the barycenter", got)
	}

	// Every planet is inside 38 AU, so all of their masses are central.
	wantGM := sunCenter.GM
	for _, p := range planetSystems {
		wantGM += p.gm
	}

	if got := fitted.CentralBody().GM; got != wantGM {
		t.Errorf("central mass %v m³/s², want the Sun and all eight planet systems, %v", got, wantGM)
	}

	if !fitted.Epoch().Equal(el.Epoch()) {
		t.Errorf("refit osculates at %v, want the given epoch %v", fitted.Epoch(), el.Epoch())
	}

	helio, helioVel, err := el.StateAt(centerEpoch)
	if err != nil {
		t.Fatal(err)
	}

	sun, sunVel, err := barycentric(&sofaBase{}, core.Sun, centerEpoch)
	if err != nil {
		t.Fatal(err)
	}

	bary, baryVel, err := fitted.StateAt(centerEpoch)
	if err != nil {
		t.Fatal(err)
	}

	if bary != helio.Add(sun) || baryVel != helioVel.Add(sunVel) {
		t.Errorf("refit starts at %v, %v; want the given orbit's state plus the Sun's, %v, %v",
			bary, baryVel, helio.Add(sun), helioVel.Add(sunVel))
	}
}

var errNoSun = errors.New("this base has no Sun")

// sunBase is the SOFA base with its Sun moved by offset, or failing with err.
type sunBase struct {
	sofaBase

	offset vector.Vec3
	err    error
	asked  int
}

func (b *sunBase) Locate(id core.ID, t time.Time) (core.State, error) {
	if id != core.Sun {
		return b.sofaBase.Locate(id, t)
	}

	b.asked++

	if b.err != nil {
		return core.State{}, b.err
	}

	st, err := b.sofaBase.Locate(id, t)
	st.Pos = st.Pos.Add(b.offset)

	return st, err
}

// TestRegisterRefitsThroughTheBaseSun: the refit is placed about the base's
// barycenter, the one ephemeris.At meets it at, so a base whose Sun is not
// SOFA's moves the refit with it. The choice reads SOFA and does not move.
func TestRegisterRefitsThroughTheBaseSun(t *testing.T) {
	t.Parallel()

	el := orbitAt(t, 45.6, 0.155)

	// 116 km, SOFA's Sun against Horizons', give or take.
	base := &sunBase{offset: vector.V3(7.0e-7, 3.3e-7, 0.5e-7)}

	p := New(WithBase(base))
	if err := p.Register(1_000_000, el); err != nil {
		t.Fatalf("Register: %v", err)
	}

	fitted := p.elements[1_000_000]

	if got, want := fitted.CentralBody(), registered(t, el).CentralBody(); got != want {
		t.Errorf("fitted about %v, GM %v over the shifted base; want %v, GM %v, as over SOFA",
			got.ID, got.GM, want.ID, want.GM)
	}

	helio, _, err := el.StateAt(centerEpoch)
	if err != nil {
		t.Fatal(err)
	}

	sun, _, err := barycentric(&sofaBase{}, core.Sun, centerEpoch)
	if err != nil {
		t.Fatal(err)
	}

	got, _, err := fitted.StateAt(centerEpoch)
	if err != nil {
		t.Fatal(err)
	}

	if want := helio.Add(sun.Add(base.offset)); got != want {
		t.Errorf("refit starts at %v, want the given orbit plus the base's Sun, %v", got, want)
	}
}

// TestRegisterNeedsTheBaseSunOnlyToRefit: a base that cannot place the Sun
// fails Register for an orbit refit about the barycenter, since nothing would
// place it there, and is never asked for one kept about the Sun.
func TestRegisterNeedsTheBaseSunOnlyToRefit(t *testing.T) {
	t.Parallel()

	base := &sunBase{err: errNoSun}
	p := New(WithBase(base))

	if err := p.Register(1, orbitAt(t, 2.77, 0.079)); err != nil {
		t.Errorf("Register kept about the Sun: %v", err)
	}

	if base.asked != 0 {
		t.Errorf("the base was asked for the Sun %d times for an orbit kept about the Sun", base.asked)
	}

	if err := p.Register(2, orbitAt(t, 45.6, 0.155)); !errors.Is(err, errNoSun) {
		t.Errorf("Register refit about the barycenter: %v, want the base's %v", err, errNoSun)
	}

	if _, ok := p.elements[2]; ok {
		t.Error("Register stored an orbit it could not place")
	}
}

// TestRegisterKeepsWhatIsNotAHeliocentricOrbit: a satellite's set, and a
// heliocentric set given a satellite modifier, are stored as given.
func TestRegisterKeepsWhatIsNotAHeliocentricOrbit(t *testing.T) {
	t.Parallel()

	far := orbitAt(t, 45.6, 0.155)

	jupiter := CentralBody{ID: core.JupiterBarycenter, GM: constants.Ephemeris.JupiterSystemGravitationalParameter.Value}

	for name, el := range map[string]Elements{
		"a satellite":          far.WithCentralBody(jupiter),
		"a period":             far.WithPeriod(100_000),
		"a secular precession": far.WithSecularPrecession(SecularPrecession{ApsisPeriod: 1e6}),
		"a Laplace plane":      far.WithLaplacePlane(LaplacePlane{RA: angle.Deg(268), Dec: angle.Deg(64.5)}),
	} {
		fitted := registered(t, el)
		if fitted.state != nil || fitted.CentralBody() != el.CentralBody() {
			t.Errorf("%s: refit about %v, want it kept as given", name, fitted.CentralBody().ID)
		}
	}
}

// TestCenterChoiceCrossesOverBeyondJupiter logs what each fit leaves out across
// distances, and holds the crossover between the main belt and Saturn's orbit,
// where the study (#803) found the better fit changes.
func TestCenterChoiceCrossesOverBeyondJupiter(t *testing.T) {
	t.Parallel()

	var crossover float64

	for _, a := range []float64{1, 2.7, 4, 5.2, 7, 10, 20, 30, 45, 90} {
		choice, err := chooseCenter(orbitAt(t, a, 0.05))
		if err != nil {
			t.Fatal(err)
		}

		t.Logf("a = %4.1f AU: helio %.3g, bary %.3g, ratio %.3g", a, choice.helio, choice.bary, choice.bary/choice.helio)

		if choice.bary < choice.helio && crossover == 0 {
			crossover = a
		}
	}

	if !(crossover > 2.7 && crossover <= 10) {
		t.Errorf("the barycentric fit first leaves out less at a = %v AU; want beyond the main belt and by Saturn's orbit", crossover)
	}
}
