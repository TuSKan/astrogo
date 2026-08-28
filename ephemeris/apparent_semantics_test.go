package ephemeris_test

import (
	"math"
	"testing"
	gotime "time"

	eph "github.com/TuSKan/astrogo/ephemeris"
	astrotime "github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// lightSpeedAUPerDay is the constant ApparentState iterates against.
const lightSpeedAUPerDay = 173.144632674

// orbitingProvider models what a real ephemeris provider does and the existing
// mock deliberately does not: it holds two bodies on their own orbits about
// the barycentre and returns the geocentric state as the difference of the
// two, both evaluated at the requested epoch.
//
// That difference is the whole point. mockLinearProvider returns a geocentric
// vector that is a function of time on its own, so iterating it retards the
// target and nothing else. A real provider retards the observer along with it,
// because the Earth's own barycentric position is looked up at the same epoch.
type orbitingProvider struct {
	epoch astrotime.Time
}

// earthAt and marsAt are circular coplanar orbits, which is enough: the
// question is about which epoch each body is evaluated at, not about orbital
// mechanics.
func earthAt(days float64) (pos, vel vector.Vec3) {
	const (
		radius = 1.0
		period = 365.25
	)

	w := 2 * math.Pi / period
	s, c := math.Sincos(w * days)

	return vector.V3(radius*c, radius*s, 0),
		vector.V3(-radius*w*s, radius*w*c, 0)
}

func marsAt(days float64) (pos, vel vector.Vec3) {
	const (
		radius = 1.5
		period = 686.98
		// Zero puts the two at opposition at the epoch, where the Earth's
		// velocity is square to the line of sight and the aberration term is
		// therefore at its largest. Aberration displaces by the component of
		// v_earth perpendicular to the sight line, so a geometry that happens
		// to look along the Earth's motion would show almost none of it and
		// make this test blind to the thing it is checking.
		phase = 0
	)

	w := 2 * math.Pi / period
	s, c := math.Sincos(w*days + phase)

	return vector.V3(radius*c, radius*s, 0),
		vector.V3(-radius*w*s, radius*w*c, 0)
}

func (p *orbitingProvider) State(_ eph.ID, t astrotime.Time) (eph.State, error) {
	d := p.days(t)

	mp, mv := marsAt(d)
	ep, ev := earthAt(d)

	return eph.State{Pos: mp.Sub(ep), Vel: mv.Sub(ev)}, nil
}

func (p *orbitingProvider) Close() error { return nil }

// days is the offset from the provider's epoch, differenced through the
// two-part Julian date so a light time of a few hundred seconds is not lost
// to the representation.
func (p *orbitingProvider) days(t astrotime.Time) float64 {
	j1, j2 := t.JDParts()
	b1, b2 := p.epoch.JDParts()

	return (j1 - b1) + (j2 - b2)
}

func arcsecBetween(a, b vector.Vec3) float64 {
	dot := a.Dot(b) / (a.Norm() * b.Norm())

	return math.Acos(math.Min(1, math.Max(-1, dot))) * 180 / math.Pi * 3600
}

// ApparentState returns the apparent place — light time and annual aberration
// together — not the astrometric place.
//
// This is a consequence of how a provider defines a geocentric state rather
// than of the iteration itself. State(target, t') is target(t') - earth(t'),
// so asking for it at the retarded epoch moves the Earth back as well as the
// target. The astrometric vector wants the target retarded and the observer
// left where it is, so the result differs from it by earth(t) - earth(t-tau),
// which is v_earth * tau. Since tau is the range over c, that term is the
// range times v_earth/c — which is exactly the first-order aberration
// displacement, in the right direction.
//
// So the two errors cancel into the right answer, and the function is correct
// as an apparent place. The reason this is worth pinning down is that its own
// doc comment used to call the result geometric, and a caller who believed
// that and applied aberration themselves would double it: for a planet at
// opposition that is a twenty arcsecond mistake, which is a hundred times any
// tolerance an astrometric pipeline works to.
func TestApparentStateIsTheApparentPlaceNotTheAstrometricOne(t *testing.T) {
	t.Parallel()

	epoch := astrotime.FromGo(gotime.Date(2026, 8, 21, 0, 0, 0, 0, gotime.UTC))
	provider := &orbitingProvider{epoch: epoch}

	got, err := eph.ApparentState(provider, eph.Mars, epoch)
	if err != nil {
		t.Fatalf("ApparentState: %v", err)
	}

	// The reference computation, done the textbook way and entirely
	// independently of the code under test.
	earthPos, earthVel := earthAt(0)

	// Solve tau from |mars(-tau) - earth(0)| = c * tau.
	tau := 0.0

	for range 60 {
		marsPos, _ := marsAt(-tau)
		tau = marsPos.Sub(earthPos).Norm() / lightSpeedAUPerDay
	}

	marsRetarded, _ := marsAt(-tau)
	astrometric := marsRetarded.Sub(earthPos)

	// First-order annual aberration: the direction is displaced toward the
	// apex of the observer's motion by v/c.
	aberration := earthVel.MulScalar(astrometric.Norm() / lightSpeedAUPerDay)
	apparent := astrometric.Add(aberration)

	// What ApparentState returns must be the apparent place.
	if sep := arcsecBetween(got.Pos, apparent); sep > 0.05 {
		t.Errorf("ApparentState is %.4f arcsec from the apparent place; it should be the apparent place",
			sep)
	}

	// And it must be measurably away from the astrometric place, so that a
	// caller cannot mistake one for the other by accident.
	sepAstrometric := arcsecBetween(got.Pos, astrometric)
	if sepAstrometric < 5 {
		t.Errorf("ApparentState is only %.4f arcsec from the astrometric place, so this test is "+
			"no longer distinguishing the two", sepAstrometric)
	}

	// The gap between them is the aberration displacement, which is the
	// component of v_earth perpendicular to the line of sight, over c. At
	// opposition that is very nearly the whole of it — the constant of
	// aberration, 20.5 arcsec — but it is predicted from the geometry here
	// rather than assumed, so the test stays honest if the orbits are changed.
	sight := astrometric.MulScalar(1 / astrometric.Norm())
	perpendicular := earthVel.Sub(sight.MulScalar(earthVel.Dot(sight)))
	wantArcsec := perpendicular.Norm() / lightSpeedAUPerDay * 180 / math.Pi * 3600

	if math.Abs(sepAstrometric-wantArcsec) > 0.05 {
		t.Errorf("apparent and astrometric differ by %.4f arcsec, want %.4f from the perpendicular "+
			"component of the Earth's velocity over c", sepAstrometric, wantArcsec)
	}

	// At opposition that displacement is the constant of aberration, which is
	// a number from outside this calculation.
	if math.Abs(wantArcsec-20.49) > 0.5 {
		t.Errorf("at opposition the aberration term is %.4f arcsec, want about 20.49 — the "+
			"geometry is no longer the one this test was built on", wantArcsec)
	}

	// The light time is real rather than negligible, so the iteration is
	// genuinely being exercised. At opposition Mars is half an astronomical
	// unit away, which is about 250 seconds of it.
	if seconds := tau * 86400; seconds < 100 {
		t.Errorf("the light time is only %.1f seconds; the geometry is too close to test a retardation",
			seconds)
	}
}

// A body that does not move has no light-time correction and no aberration to
// apply, so the apparent place is the geometric one whatever the range.
func TestApparentStateWithNoMotionIsGeometric(t *testing.T) {
	t.Parallel()

	epoch := astrotime.FromGo(gotime.Date(2026, 8, 21, 0, 0, 0, 0, gotime.UTC))

	still := &staticProvider{pos: vector.V3(3.5, -1.2, 0.4)}

	got, err := eph.ApparentState(still, eph.Jupiter, epoch)
	if err != nil {
		t.Fatalf("ApparentState: %v", err)
	}

	if d := got.Pos.Sub(still.pos).Norm(); d > 1e-15 {
		t.Errorf("a motionless body moved by %g AU under the light-time reduction", d)
	}
}

type staticProvider struct{ pos vector.Vec3 }

func (p *staticProvider) State(_ eph.ID, _ astrotime.Time) (eph.State, error) {
	return eph.State{Pos: p.pos, Vel: vector.Zero()}, nil
}

func (p *staticProvider) Close() error { return nil }
