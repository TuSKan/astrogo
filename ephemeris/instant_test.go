package ephemeris_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// errProviderFailed is what a scene returns for a body it is told to fail
// on, so a test can assert the wrapping kept it.
var errProviderFailed = errors.New("provider refused")

// errNotInScene is what a scene returns for a body it does not hold.
var errNotInScene = errors.New("no such body in the scene")

// sceneLight is c in AU/day, from the light time of one AU, as the code
// under test derives it.
const sceneLight = 86400.0 / 499.00478383615643

// scene is a synthetic solar system that answers as a real provider does:
// each body about the center it is computed from, on a function of time.
type scene struct {
	epoch  time.Time
	bodies map[eph.ID]sceneBody
	frame  eph.Frame
	fail   func(id eph.ID, t time.Time) bool
	calls  map[eph.ID]int
}

// sceneBody is one body of a scene: where it is about its center, days after
// the scene's epoch.
type sceneBody struct {
	center eph.ID
	at     func(days float64) (pos, vel vector.Vec3)
}

func (s *scene) Locate(id eph.ID, t time.Time) (eph.State, error) {
	if s.calls != nil {
		s.calls[id]++
	}

	if s.fail != nil && s.fail(id, t) {
		return eph.State{}, errProviderFailed
	}

	b, ok := s.bodies[id]
	if !ok {
		return eph.State{}, errNotInScene
	}

	frame := s.frame
	if frame == eph.FrameUnspecified {
		frame = eph.FrameICRS
	}

	// Differenced through the two-part Julian date, so a light time of
	// seconds is not lost to the representation.
	j1, j2 := t.JDParts()
	b1, b2 := s.epoch.JDParts()
	pos, vel := b.at((j1 - b1) + (j2 - b2))

	return eph.State{Pos: pos, Vel: vel, Frame: frame, Center: b.center}, nil
}

func (s *scene) Close() error { return nil }

// still is a body at rest at pos about center.
func still(center eph.ID, pos vector.Vec3) sceneBody {
	return sceneBody{center: center, at: func(float64) (vector.Vec3, vector.Vec3) { return pos, vector.Zero() }}
}

// circling is a body on a circle in the xy plane about center.
func circling(center eph.ID, radius, periodDays, phase float64) sceneBody {
	return sceneBody{center: center, at: func(days float64) (vector.Vec3, vector.Vec3) {
		w := 2 * math.Pi / periodDays
		s, c := math.Sincos(w*days + phase)

		return vector.V3(radius*c, radius*s, 0), vector.V3(-radius*w*s, radius*w*c, 0)
	}}
}

// The vectors of kernelScene, as a JPL kernel nests them: the Moon and the
// Earth about the Earth-Moon barycenter, everything else about the Solar
// System barycenter.
var (
	sceneEMB   = vector.V3(0.98, 0.21, 0.013)
	sceneEarth = vector.V3(-1.2e-5, 2.1e-5, 0.4e-5)
	sceneMoon  = vector.V3(0.0024, -0.0011, 0.0002)
	sceneSun   = vector.V3(0.0031, 0.0012, -0.0001)
	sceneMars  = vector.V3(1.31, 0.82, 0.021)
)

func kernelScene() *scene {
	epoch := time.Date(2026, time.June, 21, 0, 0, 0, 0, time.LocationUTC)

	return &scene{
		epoch: epoch,
		calls: map[eph.ID]int{},
		bodies: map[eph.ID]sceneBody{
			eph.EarthMoonBarycenter: still(eph.SolarSystemBarycenter, sceneEMB),
			eph.Earth:               still(eph.EarthMoonBarycenter, sceneEarth),
			eph.Moon:                still(eph.EarthMoonBarycenter, sceneMoon),
			eph.Sun:                 still(eph.SolarSystemBarycenter, sceneSun),
			eph.MarsBarycenter:      still(eph.SolarSystemBarycenter, sceneMars),
		},
	}
}

// TestGeometricMeetsAtTheCommonCenter: a state about a center is the two
// bodies' chains differenced below the first body both pass through, and
// nothing above it is located. The Moon about the Earth in a kernel is the
// Moon about the Earth-Moon barycenter less the Earth about it, exactly, and
// the barycenter itself, an AU away, never enters.
func TestGeometricMeetsAtTheCommonCenter(t *testing.T) {
	t.Parallel()

	s := kernelScene()
	in := eph.At(s, s.epoch)

	for _, c := range []struct {
		name           string
		target, center eph.ID
		want           vector.Vec3
	}{
		{"the Moon about the Earth", eph.Moon, eph.Earth, sceneMoon.Sub(sceneEarth)},
		{"the Earth about the Moon", eph.Earth, eph.Moon, sceneEarth.Sub(sceneMoon)},
		{"Mars about the Earth", eph.MarsBarycenter, eph.Earth, sceneMars.Sub(sceneEarth.Add(sceneEMB))},
		{"the Earth about the Sun", eph.Earth, eph.Sun, sceneEarth.Add(sceneEMB).Sub(sceneSun)},
		{"the Moon about the barycenter", eph.Moon, eph.SolarSystemBarycenter, sceneMoon.Add(sceneEMB)},
		{"Mars about itself", eph.MarsBarycenter, eph.MarsBarycenter, vector.Zero()},
	} {
		st, err := in.Geometric(c.target, c.center)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}

		if st.Pos != c.want || st.Center != c.center || st.Frame != eph.FrameICRS {
			t.Errorf("%s: %v about %v on %s, want %v about %v exactly", c.name, st.Pos, st.Center, st.Frame, c.want, c.center)
		}
	}

	if n := s.calls[eph.EarthMoonBarycenter]; n != 1 {
		t.Errorf("the Earth-Moon barycenter was located %d times; only the chains through it, Mars about the Earth's and the Moon about the barycenter, need it, and once", n)
	}
}

// TestGeometricLeavesTheBarycenterOutOfTheMoon is the half of the test above
// the memo would hide: alone, the Moon about the Earth locates two bodies.
func TestGeometricLeavesTheBarycenterOutOfTheMoon(t *testing.T) {
	t.Parallel()

	s := kernelScene()

	if _, err := eph.At(s, s.epoch).Geometric(eph.Moon, eph.Earth); err != nil {
		t.Fatal(err)
	}

	if len(s.calls) != 2 || s.calls[eph.Moon] != 1 || s.calls[eph.Earth] != 1 {
		t.Errorf("the Moon about the Earth located %v; want the Moon and the Earth, once each", s.calls)
	}
}

// TestAnInstantLocatesEachBodyOnce: everything an epoch needs, from one
// Instant, locates each body once.
func TestAnInstantLocatesEachBodyOnce(t *testing.T) {
	t.Parallel()

	s := kernelScene()
	in := eph.At(s, s.epoch)

	for _, pair := range [][2]eph.ID{
		{eph.Moon, eph.Earth}, {eph.MarsBarycenter, eph.Earth}, {eph.Sun, eph.Earth},
		{eph.Earth, eph.SolarSystemBarycenter}, {eph.MarsBarycenter, eph.Sun},
	} {
		if _, err := in.Geometric(pair[0], pair[1]); err != nil {
			t.Fatal(err)
		}
	}

	for id, n := range s.calls {
		if n != 1 {
			t.Errorf("%v was located %d times at one epoch", id, n)
		}
	}
}

// TestGeometricComposesOnlyTheICRSAxes: the GCRS shares the ICRS's axes and
// composes with it, as a satellite about the Earth must with the Earth about
// the Sun; TEME and an unlabeled state do not.
func TestGeometricComposesOnlyTheICRSAxes(t *testing.T) {
	t.Parallel()

	s := kernelScene()

	satellite := eph.ID(-125544)
	s.bodies[satellite] = still(eph.Earth, vector.V3(4e-5, 0, 0))

	gcrs := &scene{epoch: s.epoch, frame: eph.FrameGCRS, bodies: map[eph.ID]sceneBody{satellite: s.bodies[satellite]}}
	mixed := &mixedProvider{icrs: s, gcrs: gcrs, sat: satellite}

	st, err := eph.At(mixed, s.epoch).Geometric(satellite, eph.Sun)
	if err != nil {
		t.Fatalf("a GCRS satellite about the Sun: %v", err)
	}

	if want := vector.V3(4e-5, 0, 0).Add(sceneEarth).Add(sceneEMB).Sub(sceneSun); st.Pos.Sub(want).Norm() > 1e-16 {
		t.Errorf("a GCRS satellite about the Sun is %v, want %v", st.Pos, want)
	}

	for _, frame := range []eph.Frame{eph.FrameTEME, eph.FrameITRS} {
		bad := kernelScene()
		bad.frame = frame

		if _, err := eph.At(bad, bad.epoch).Geometric(eph.Moon, eph.Earth); !errors.Is(err, eph.ErrWrongFrame) {
			t.Errorf("states on %s axes composed: err %v, want ErrWrongFrame", frame, err)
		}
	}
}

// mixedProvider answers one satellite in the GCRS and everything else in the
// ICRS.
type mixedProvider struct {
	icrs, gcrs *scene
	sat        eph.ID
}

func (m *mixedProvider) Locate(id eph.ID, t time.Time) (eph.State, error) {
	if id == m.sat {
		return m.gcrs.Locate(id, t)
	}

	return m.icrs.Locate(id, t)
}

func (m *mixedProvider) Close() error { return nil }

// TestGeometricRefusesCentersThatLoop: a provider whose centers never reach
// the barycenter is refused rather than walked forever.
func TestGeometricRefusesCentersThatLoop(t *testing.T) {
	t.Parallel()

	s := kernelScene()
	a, b := eph.ID(2000001), eph.ID(2000002)
	s.bodies[a] = still(b, vector.V3(1, 0, 0))
	s.bodies[b] = still(a, vector.V3(-1, 0, 0))

	if _, err := eph.At(s, s.epoch).Geometric(a, eph.Earth); !errors.Is(err, eph.ErrChainTooDeep) {
		t.Errorf("centers that loop: err %v, want ErrChainTooDeep", err)
	}
}

// TestGeometricReportsAMissingBody keeps the provider's own error.
func TestGeometricReportsAMissingBody(t *testing.T) {
	t.Parallel()

	s := kernelScene()

	st, err := eph.At(s, s.epoch).Geometric(eph.JupiterBarycenter, eph.Earth)
	if !errors.Is(err, errNotInScene) || st != (eph.State{}) {
		t.Errorf("a body the provider does not hold: %+v, err %v", st, err)
	}
}

// TestGeometricAllocatesNothing is an allocation contract (CLAUDE.md): the
// Instant's chains and memo live in the Instant, so composing a state, the
// innermost loop of every event search, allocates nothing.
func TestGeometricAllocatesNothing(t *testing.T) {
	s := kernelScene()
	s.calls = nil

	allocs := testing.AllocsPerRun(100, func() {
		in := eph.At(s, s.epoch)
		if _, err := in.Geometric(eph.MarsBarycenter, eph.Earth); err != nil {
			t.Fatal(err)
		}

		if _, err := in.Geometric(eph.Moon, eph.Earth); err != nil {
			t.Fatal(err)
		}
	})

	if allocs != 0 {
		t.Errorf("an Instant and two geometric states allocate %v times", allocs)
	}
}

// orbitingScene puts the Sun at the barycenter and the Earth and Mars on
// circles about it, at opposition at the epoch: there the Earth's velocity is
// square to the line of sight, and aberration is at its largest.
func orbitingScene() *scene {
	return &scene{
		epoch: time.Date(2026, time.August, 21, 0, 0, 0, 0, time.LocationUTC),
		bodies: map[eph.ID]sceneBody{
			eph.Sun:            still(eph.SolarSystemBarycenter, vector.Zero()),
			eph.Earth:          circling(eph.Sun, 1, 365.25, 0),
			eph.MarsBarycenter: circling(eph.Sun, 1.5, 686.98, 0),
		},
	}
}

// arcsecBetween is the angle between two directions, from atan2 of their
// cross and dot products: acos of the dot product cannot resolve an angle
// below about 2e-8 rad, 0.004″, which is most of what these tests measure.
func arcsecBetween(a, b vector.Vec3) float64 {
	return math.Atan2(a.Cross(b).Norm(), a.Dot(b)) * 180 / math.Pi * 3600
}

// TestApparentIsTheTextbookPlace holds the Instant's places to a computation
// done the textbook way, entirely apart from the code under test: the light
// time solved from |mars(−τ) − earth(0)| = cτ, the astrometric vector, and
// the first-order aberration toward the apex of the Earth's motion.
//
// The astrometric place must be that solution, and the apparent place within
// 0.05″ of the aberrated one (iauAb is relativistic and the Sun deflects the
// ray a little; neither reaches that). And the two must be apart by the
// aberration the geometry predicts, so that neither can stand in for the other:
// handed to a consumer that aberrates itself, the apparent place would count it
// twice, twenty arcseconds at opposition.
func TestApparentIsTheTextbookPlace(t *testing.T) {
	t.Parallel()

	s := orbitingScene()
	in := eph.At(s, s.epoch)

	astrometric, err := in.Astrometric(eph.MarsBarycenter, eph.Geocenter())
	if err != nil {
		t.Fatal(err)
	}

	apparent, err := in.Apparent(eph.MarsBarycenter, eph.Geocenter())
	if err != nil {
		t.Fatal(err)
	}

	earthPos, earthVel := s.bodies[eph.Earth].at(0)

	tau := 0.0
	for range 60 {
		marsPos, _ := s.bodies[eph.MarsBarycenter].at(-tau)
		tau = marsPos.Sub(earthPos).Norm() / sceneLight
	}

	marsThen, _ := s.bodies[eph.MarsBarycenter].at(-tau)
	want := marsThen.Sub(earthPos)

	if d := astrometric.Pos.Sub(want).Norm(); d > 1e-12 {
		t.Errorf("the astrometric place is %g AU from the light-time solution", d)
	}

	aberrated := want.Add(earthVel.MulScalar(want.Norm() / sceneLight))
	if sep := arcsecBetween(apparent.Pos, aberrated); sep > 0.05 {
		t.Errorf("the apparent place is %.4f″ from the aberrated astrometric one", sep)
	}

	sight := want.Unit()
	perpendicular := earthVel.Sub(sight.MulScalar(earthVel.Dot(sight)))
	wantArcsec := perpendicular.Norm() / sceneLight * 180 / math.Pi * 3600

	if got := arcsecBetween(apparent.Pos, astrometric.Pos); math.Abs(got-wantArcsec) > 0.05 || math.Abs(wantArcsec-20.49) > 0.5 {
		t.Errorf("the apparent and astrometric places are %.4f″ apart, want %.4f″, the constant of aberration at opposition", got, wantArcsec)
	}

	if seconds := tau * 86400; seconds < 100 {
		t.Errorf("the light time is only %.1f s; the geometry no longer exercises a retardation", seconds)
	}
}

// staticScene holds the Earth and a target still, the Sun at the barycenter
// a quarter-turn from the target as the Earth sees it, so the only thing
// between the target's apparent place and its geometric one is the Sun's
// deflection of the light: a few milliarcseconds at that elongation.
func staticScene(target vector.Vec3) *scene {
	earth := vector.V3(-0.322, -0.940, 0)

	return &scene{
		epoch: time.Date(2026, time.August, 21, 0, 0, 0, 0, time.LocationUTC),
		bodies: map[eph.ID]sceneBody{
			eph.Sun:               still(eph.SolarSystemBarycenter, vector.Zero()),
			eph.Earth:             still(eph.Sun, earth),
			eph.JupiterBarycenter: still(eph.Sun, target.Add(earth)),
		},
	}
}

// TestApparentOfAMotionlessBodyIsDeflectionAlone: nothing moves, so there is
// no light-time correction and no aberration, and the apparent place is the
// geometric one deflected by the Sun, as iauLd deflects it, with its range.
func TestApparentOfAMotionlessBodyIsDeflectionAlone(t *testing.T) {
	t.Parallel()

	geocentric := vector.V3(3.5, -1.2, 0.4)
	s := staticScene(geocentric)

	got, err := eph.At(s, s.epoch).Apparent(eph.JupiterBarycenter, eph.Geocenter())
	if err != nil {
		t.Fatal(err)
	}

	earth, _ := s.bodies[eph.Earth].at(0)
	helio := geocentric.Add(earth)
	arr := func(v vector.Vec3) [3]float64 { u := v.Unit(); return [3]float64{u.X, u.Y, u.Z} }
	em := earth.Norm()
	d := gofaext.Ld(1, arr(geocentric), arr(helio), arr(earth), em, 1e-6/math.Max(em*em, 1))
	want := vector.V3(d[0], d[1], d[2]).Unit().MulScalar(geocentric.Norm())

	if diff := got.Pos.Sub(want).Norm(); diff > 1e-15*want.Norm() {
		t.Errorf("a motionless body is %g AU from its deflected geometric place", diff)
	}

	if mas := arcsecBetween(want, geocentric) * 1000; mas < 0.1 || mas > 100 {
		t.Errorf("the deflection at a quarter-turn from the Sun is %.3f mas, outside the milliarcseconds it is", mas)
	}
}

// TestApparentDoesNotDeflectTheSun: a body does not bend its own light. With
// the observer still, so there is no aberration either, the Sun's apparent
// direction is its astrometric one, while a planet beside it, the positive
// control, is deflected: without that, the Sun's case would pass with the
// deflection deleted.
func TestApparentDoesNotDeflectTheSun(t *testing.T) {
	t.Parallel()

	// Twelve degrees from the Sun, half an AU from it.
	s := staticScene(vector.V3(0.75, 1.25, 0.05))
	in := eph.At(s, s.epoch)

	for _, c := range []struct {
		id        eph.ID
		deflected bool
	}{{eph.Sun, false}, {eph.JupiterBarycenter, true}} {
		astrometric, err := in.Astrometric(c.id, eph.Geocenter())
		if err != nil {
			t.Fatal(err)
		}

		apparent, err := in.Apparent(c.id, eph.Geocenter())
		if err != nil {
			t.Fatal(err)
		}

		sep := arcsecBetween(apparent.Pos, astrometric.Pos)

		switch {
		case c.deflected && sep < 1e-4:
			t.Errorf("%v beside the Sun is not deflected (%.2g″)", c.id, sep)
		case !c.deflected && sep > 1e-9:
			t.Errorf("the Sun is deflected by its own light, %.2g″", sep)
		}

		if got, want := apparent.Pos.Norm(), astrometric.Pos.Norm(); math.Abs(got-want) > 1e-15*want {
			t.Errorf("%v: the range changed from %.15g to %.15g AU; deflection and aberration bend a ray", c.id, want, got)
		}
	}
}

// TestPlacesReportWhichFetchFailed: an apparent place reads the observer, the
// target now and retarded, and the Sun now and retarded, and a failure of each
// must say which and keep the provider's error, and leave the zero State, never
// a partial one: State{} is a legitimate-looking answer, a body at rest at the
// geocentre.
func TestPlacesReportWhichFetchFailed(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name  string
		body  eph.ID
		atObs bool
		says  string
	}{
		{"the observer", eph.Earth, true, "the observer"},
		{"the target at the observing epoch", eph.MarsBarycenter, true, "at the observing epoch"},
		{"the target retarded", eph.MarsBarycenter, false, "retarded by light time"},
		{"the Sun at the observing epoch", eph.Sun, true, "the Sun for light deflection"},
		{"the Sun retarded", eph.Sun, false, "the Sun at the retarded epoch"},
	} {
		s := kernelScene()
		s.fail = func(id eph.ID, t time.Time) bool { return id == c.body && t.Equal(s.epoch) == c.atObs }

		st, err := eph.At(s, s.epoch).Apparent(eph.MarsBarycenter, eph.Geocenter())

		if msg := errString(err); !errors.Is(err, errProviderFailed) || !strings.Contains(msg, c.says) {
			t.Errorf("%s failing: err %q, want the provider's error and %q", c.name, msg, c.says)
		}

		if st != (eph.State{}) {
			t.Errorf("%s failing: state %+v, want the zero State", c.name, st)
		}
	}
}

func errString(err error) string {
	if err == nil {
		return "<nil>"
	}

	return err.Error()
}

// TestApparentOfDegenerateGeometry: a target at the observer has no
// direction and is refused; a target at the Sun's place, and an observer at
// it, leave the deflection or its gravitational term undefined, which is
// dropped rather than turned into NaN.
func TestApparentOfDegenerateGeometry(t *testing.T) {
	t.Parallel()

	at := func(earth, target vector.Vec3) *scene {
		return &scene{
			epoch: time.Date(2026, time.June, 21, 0, 0, 0, 0, time.LocationUTC),
			bodies: map[eph.ID]sceneBody{
				eph.Sun:            still(eph.SolarSystemBarycenter, vector.Zero()),
				eph.Earth:          still(eph.Sun, earth),
				eph.MarsBarycenter: still(eph.Sun, target),
			},
		}
	}

	s := at(vector.V3(1, 0, 0), vector.V3(1, 0, 0))
	if _, err := eph.At(s, s.epoch).Apparent(eph.MarsBarycenter, eph.Geocenter()); !errors.Is(err, eph.ErrZeroVector) {
		t.Errorf("a target at the observer: err %v, want ErrZeroVector", err)
	}

	for name, s := range map[string]*scene{
		"a target at the Sun's place": at(vector.V3(1, 0, 0), vector.Zero()),
		"an observer at the Sun":      at(vector.Zero(), vector.V3(1.5, 0.2, 0)),
	} {
		st, err := eph.At(s, s.epoch).Apparent(eph.MarsBarycenter, eph.Geocenter())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		if p := st.Pos; math.IsNaN(p.X+p.Y+p.Z) || math.IsInf(p.X+p.Y+p.Z, 0) || p.Norm() == 0 {
			t.Errorf("%s: %+v", name, p)
		}
	}
}

// TestAstrometricHasConverged checks the light-time loop's exit condition
// against what it stands for: one more pass, done here by hand, must not move
// the answer by a microarcsecond, for the fastest relative motions there are.
//
// Except the Moon's, by what the analytical series can resolve. SOFA reads
// time as (d1 − J2000) + d2, which near JD 2.46e6 resolves 1.6e-12 days,
// 0.16 µs, and the light time is solved about the barycenter, where the Moon
// moves with the Earth at 30 km/s: one quantum is 4.7 mm, 2.7 µas at perigee,
// and a pass can land in the next one. It was invisible when the light time
// was solved about the Earth, where the Moon moves 1 km/s.
func TestAstrometricHasConverged(t *testing.T) {
	t.Parallel()

	prov := eph.Default()

	for _, id := range []eph.ID{eph.Moon, eph.Sun, eph.Venus, eph.MarsBarycenter, eph.JupiterBarycenter, eph.NeptuneBarycenter} {
		var worst float64

		for d := 0; d < 365; d += 7 {
			tm := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC).Add(unit.Days(float64(d)))

			settled, err := eph.At(prov, tm).Astrometric(id, eph.Geocenter())
			if err != nil {
				t.Fatal(err)
			}

			tau := settled.Pos.Norm() / sceneLight

			target, err := eph.At(prov, tm.Add(unit.Days(-tau))).Geometric(id, eph.SolarSystemBarycenter)
			if err != nil {
				t.Fatal(err)
			}

			earth, err := eph.At(prov, tm).Geometric(eph.Earth, eph.SolarSystemBarycenter)
			if err != nil {
				t.Fatal(err)
			}

			worst = math.Max(worst, arcsecBetween(target.Pos.Sub(earth.Pos), settled.Pos))
		}

		bound := 1e-6
		if id == eph.Moon {
			bound = 3e-6
		}

		if worst > bound {
			t.Errorf("one more light-time pass moves %v by %.3g″, over %.0g″", id, worst, bound)
		}
	}
}
