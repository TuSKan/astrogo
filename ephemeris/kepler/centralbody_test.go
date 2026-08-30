package kepler_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/kepler"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// auMeters is the astronomical unit, for turning a satellite's semi-major
// axis in kilometres into the astronomical units the elements take.
var auMeters = constants.IAU.AstronomicalUnit.Value

// kmToAU converts a distance in kilometres.
//
// A satellite's semi-major axis in AU is a very small number — Io's is
// 2.8e-06 — which is a fine thing for a float64 to hold and an awkward thing
// to type, so the tests below convert from the kilometres the literature
// publishes.
func kmToAU(km float64) float64 { return km * 1e3 / auMeters }

// circular builds a circular, equatorial orbit of the given radius about
// body, which is the simplest thing whose period can be predicted in closed
// form.
func circular(t *testing.T, body kepler.CentralBody, radiusKM float64) kepler.Elements {
	t.Helper()

	el, err := kepler.NewElements(time.J2000, kmToAU(radiusKM), 0,
		angle.Deg(0), angle.Deg(0), angle.Deg(0), angle.Deg(0))
	if err != nil {
		t.Fatalf("NewElements: %v", err)
	}

	return el.WithCentralBody(body)
}

// periodDays measures the orbital period by accumulating the angle swept in
// the orbital plane until it reaches a full turn.
//
// The obvious bisection does not work here and the first version of this
// helper used it: the angle between the start position and the current one,
// via atan2, wraps at half a revolution, so it is not monotonic over an orbit
// and a bisection on it converges to whichever bracket end it started nearest.
// It reported half a year for a 1 AU heliocentric orbit and 1.5 periods for a
// Jovian one — both wrong, in different directions, which is what gave it
// away.
//
// The plane's own basis avoids that. u along the initial radius, w along the
// angular momentum, v completing the right-handed set; the angle is then
// unwrapped by accumulation rather than inferred from a single sample.
func periodDays(t *testing.T, el kepler.Elements, guessDays float64) float64 {
	t.Helper()

	r0, v0, err := el.StateAt(el.Epoch())
	if err != nil {
		t.Fatalf("StateAt(epoch): %v", err)
	}

	u := r0.Unit()
	w := r0.Cross(v0).Unit()
	v := w.Cross(u)

	const steps = 4096

	step := 2 * guessDays / steps

	var swept, prev float64

	for i := 1; i <= steps; i++ {
		when := el.Epoch().AddDays(float64(i) * step)

		r, _, serr := el.StateAt(when)
		if serr != nil {
			t.Fatalf("StateAt: %v", serr)
		}

		phi := math.Atan2(r.Dot(v), r.Dot(u))

		d := phi - prev
		for d < -math.Pi {
			d += 2 * math.Pi
		}

		for d > math.Pi {
			d -= 2 * math.Pi
		}

		if swept+d >= 2*math.Pi {
			// Linear interpolation across the step that completes the turn.
			frac := (2*math.Pi - swept) / d

			return (float64(i-1) + frac) * step
		}

		swept += d
		prev = phi
	}

	t.Fatalf("no full revolution within %v days", 2*guessDays)

	return 0
}

// TestCentralBodyDefaultsToTheSun keeps the change invisible to the
// heliocentric path: an Elements that never names a central body must
// propagate exactly as it did before one could be named.
func TestCentralBodyDefaultsToTheSun(t *testing.T) {
	el, err := kepler.NewElements(time.J2000, 2.7658, 0.07839,
		angle.Deg(10.587), angle.Deg(80.393), angle.Deg(73.597), angle.Deg(77.372))
	if err != nil {
		t.Fatalf("NewElements: %v", err)
	}

	got := el.CentralBody()
	if got.ID != core.Sun {
		t.Errorf("default central body is %s, want Sun", got.ID)
	}

	if want := constants.IAU.SunGravitationalParameter.Value; got.GM != want {
		t.Errorf("default GM = %v, want the IAU nominal solar value %v", got.GM, want)
	}
}

// TestPeriodFollowsKeplersThirdLaw is the physics this change exists to get
// right: the period depends on the central body's mass parameter, so
// swapping the parent must change it by the square root of the mass ratio
// and nothing else.
//
// Checked against the third law rather than against a published period, so
// the test does not depend on a satellite fixture and cannot pass by
// coincidence of two wrong numbers.
func TestPeriodFollowsKeplersThirdLaw(t *testing.T) {
	const radiusKM = 421_800 // roughly Io's distance, used for both

	jupiter, ok := kepler.CentralBodyFor(core.Jupiter)
	if !ok {
		t.Fatal("no central body for Jupiter")
	}

	saturn, ok := kepler.CentralBodyFor(core.Saturn)
	if !ok {
		t.Fatal("no central body for Saturn")
	}

	// T = 2π √(a³/GM), so at equal a the ratio of periods is √(GM_S/GM_J).
	aMeters := radiusKM * 1e3
	wantJup := 2 * math.Pi * math.Sqrt(aMeters*aMeters*aMeters/jupiter.GM) / 86400

	gotJup := periodDays(t, circular(t, jupiter, radiusKM), wantJup)
	if rel := math.Abs(gotJup-wantJup) / wantJup; rel > 1e-6 {
		t.Errorf("Jupiter-centred period = %.6f d, third law says %.6f d (relative %.2g)",
			gotJup, wantJup, rel)
	}

	gotSat := periodDays(t, circular(t, saturn, radiusKM), wantJup*math.Sqrt(jupiter.GM/saturn.GM))
	wantRatio := math.Sqrt(jupiter.GM / saturn.GM)

	if rel := math.Abs(gotSat/gotJup-wantRatio) / wantRatio; rel > 1e-6 {
		t.Errorf("period ratio Saturn/Jupiter = %.9f, √(GM_J/GM_S) = %.9f", gotSat/gotJup, wantRatio)
	}

	t.Logf("at a = %d km: Jupiter %.4f d, Saturn %.4f d", radiusKM, gotJup, gotSat)

	// 421,800 km is Io's semi-major axis, so this is the two-body period of
	// Io's orbit — and it is not Io's period. JPL's mean-element table gives
	// 1.762732 days; two-body gives 1.769949, ten minutes longer.
	//
	// The gap is the point rather than an error. It is Jupiter's J₂ and the
	// Laplace resonance, the perturbations this package does not model, and
	// it is why a satellite computed here is machinery rather than an
	// ephemeris. An earlier version of this test called the two-body figure
	// "Io's sidereal period", which conflated the two; the published value
	// settled it.
	const (
		twoBody       = 1.769949 // from a and GM alone
		joviPublished = 1.762732 // JPL satellite mean elements, JUP365
	)

	if math.Abs(gotJup-twoBody) > 1e-4 {
		t.Errorf("two-body period = %.6f d, want %.6f", gotJup, twoBody)
	}

	if rel := math.Abs(gotJup-joviPublished) / joviPublished; rel < 0.003 || rel > 0.005 {
		t.Errorf("two-body differs from Io's published period by %.3f%%, expected about 0.4%% — "+
			"if this shrank, something started modelling the perturbations", 100*rel)
	}
}

// TestSunCentredIsUnchangedByTheRefactor pins the heliocentric result against
// the third law too, so the shared propagation path is not quietly altered
// for the case that already worked.
func TestSunCentredIsUnchangedByTheRefactor(t *testing.T) {
	// One astronomical unit about the Sun is a year, by construction.
	el, err := kepler.NewElements(time.J2000, 1.0, 0,
		angle.Deg(0), angle.Deg(0), angle.Deg(0), angle.Deg(0))
	if err != nil {
		t.Fatalf("NewElements: %v", err)
	}

	got := periodDays(t, el, 365.25)
	if math.Abs(got-365.2) > 0.5 {
		t.Errorf("a 1 AU circular heliocentric orbit has period %.3f d, want ~365.2", got)
	}
}

// TestCentralBodyForRefusesABodyWithNoSystem keeps the helper from returning
// a zero mass parameter, which would propagate as an orbit with no gravity —
// an infinite period rather than an error.
func TestCentralBodyForRefusesABodyWithNoSystem(t *testing.T) {
	for _, id := range []core.ID{core.Sun, core.Moon, core.Mercury, core.Venus} {
		if body, ok := kepler.CentralBodyFor(id); ok {
			t.Errorf("CentralBodyFor(%s) = %+v, true; want false", id, body)
		}
	}

	for _, id := range []core.ID{core.Earth, core.Mars, core.Jupiter, core.Saturn,
		core.Uranus, core.Neptune, core.Pluto} {
		body, ok := kepler.CentralBodyFor(id)
		if !ok {
			t.Errorf("CentralBodyFor(%s) reported false", id)

			continue
		}

		if body.ID != id || body.GM <= 0 {
			t.Errorf("CentralBodyFor(%s) = %+v", id, body)
		}
	}
}

// TestCentralBodyForReturnsTheBodyParameter pins what the helper chooses,
// and the reason, which is narrower than it first looks.
//
// Two-body relative motion is governed by G(M_primary + M_satellite). The
// planet's own parameter omits the satellite and the system parameter adds
// all of them, so the two bracket the right answer rather than one being
// right. For a negligible satellite the body parameter is closer, and that
// is what this returns; for Charon it is not — see
// TestCharonNeedsTheSystemParameter.
func TestCentralBodyForReturnsTheBodyParameter(t *testing.T) {
	pluto, ok := kepler.CentralBodyFor(core.Pluto)
	if !ok {
		t.Fatal("no central body for Pluto")
	}

	if pluto.GM != constants.Ephemeris.PlutoGravitationalParameter.Value {
		t.Errorf("GM = %v, want the body parameter %v",
			pluto.GM, constants.Ephemeris.PlutoGravitationalParameter.Value)
	}
}

// TestProviderPlacesASatelliteBesideItsParent checks the composition the
// provider does: a satellite's state is its parent's plus the small offset,
// so the two must differ by the orbit's radius and no more.
func TestProviderPlacesASatelliteBesideItsParent(t *testing.T) {
	jupiter, ok := kepler.CentralBodyFor(core.Jupiter)
	if !ok {
		t.Fatal("no central body for Jupiter")
	}

	const (
		moonID = core.ID(503) // Ganymede's NAIF code, used here only as a key

		// A different radius from the third-law test on purpose: the
		// composition below must not depend on the orbit's size.
		radiusKM = 1_070_400
	)

	p := kepler.New()
	if err := p.Register(moonID, circular(t, jupiter, radiusKM)); err != nil {
		t.Fatalf("Register: %v", err)
	}

	when := time.J2000

	moon, err := p.State(moonID, when)
	if err != nil {
		t.Fatalf("State(moon): %v", err)
	}

	parent, err := p.State(core.Jupiter, when)
	if err != nil {
		t.Fatalf("State(Jupiter): %v", err)
	}

	sep := moon.Pos.Sub(parent.Pos).Norm() * auMeters / 1e3

	if math.Abs(sep-radiusKM) > 1 {
		t.Errorf("satellite is %.1f km from its parent, want %d", sep, radiusKM)
	}

	// And it must be out at Jupiter rather than near the Sun: a satellite
	// composed against the wrong body would sit an astronomical unit away,
	// which the separation check above already rules out at 0.0028 AU.

	if moon.Frame != core.FrameICRS || moon.Center != core.CenterGeocentre {
		t.Errorf("satellite state is %v/%v, want ICRS/geocentric", moon.Frame, moon.Center)
	}
}

// TestSatelliteMovesWithItsParent: over a span short against the parent's
// year but long against the satellite's month, the satellite must orbit the
// parent while the parent carries it along.
func TestSatelliteMovesWithItsParent(t *testing.T) {
	jupiter, _ := kepler.CentralBodyFor(core.Jupiter)

	const moonID = core.ID(501)

	p := kepler.New()
	if err := p.Register(moonID, circular(t, jupiter, 421_800)); err != nil {
		t.Fatalf("Register: %v", err)
	}

	var maxSep, minSep float64 = 0, math.MaxFloat64

	for day := range 40 {
		when := time.J2000.AddDays(float64(day))

		moon, err := p.State(moonID, when)
		if err != nil {
			t.Fatalf("State(moon): %v", err)
		}

		parent, perr := p.State(core.Jupiter, when)
		if perr != nil {
			t.Fatalf("State(Jupiter): %v", perr)
		}

		sep := moon.Pos.Sub(parent.Pos).Norm()
		maxSep = math.Max(maxSep, sep)
		minSep = math.Min(minSep, sep)
	}

	// A circular orbit stays at one radius no matter where the parent has
	// travelled; a composition that lost the parent term would show the
	// separation wandering by an astronomical unit.
	if spread := (maxSep - minSep) / minSep; spread > 1e-9 {
		t.Errorf("separation varied by a relative %.3g over 40 days on a circular orbit", spread)
	}
}

// TestUnregisteredBodyStillReachesTheBase keeps the new branch from
// swallowing the fall-through every other body depends on.
func TestUnregisteredBodyStillReachesTheBase(t *testing.T) {
	p := kepler.New()

	st, err := p.State(core.Mars, time.J2000)
	if err != nil {
		t.Fatalf("State(Mars): %v", err)
	}

	if st.Pos == (vector.Vec3{}) {
		t.Error("base provider returned a zero position for Mars")
	}
}

// TestPeriodOverridesTheDerivedMeanMotion is the behavioural half of
// [kepler.Elements.WithPeriod]: the supplied period has to be the one the
// mean anomaly actually advances at, not merely a field that round-trips.
//
// A period is testable without any external reference, because after exactly
// one of them the mean anomaly is back where it started and — with no apsis
// drift to move the orientation — so is the whole state. The derived
// two-body period is checked against the same instant and must *not* return,
// which is what separates an override that took effect from one silently
// ignored.
func TestPeriodOverridesTheDerivedMeanMotion(t *testing.T) {
	jupiter, ok := kepler.CentralBodyFor(core.Jupiter)
	if !ok {
		t.Fatal("no central body for Jupiter")
	}

	// Io, whose tabulated period differs from the two-body one by 0.4%.
	const ioPeriodDays = 1.762732

	derived := circular(t, jupiter, 421_800)
	if p := derived.Period(); p != 0 {
		t.Errorf("Period() = %v on elements that never set one, want 0 for derived", p)
	}

	tabulated := derived.WithPeriod(ioPeriodDays)
	if p := tabulated.Period(); p != ioPeriodDays {
		t.Errorf("Period() = %v, want %v", p, ioPeriodDays)
	}

	start, _, err := tabulated.StateAt(time.J2000)
	if err != nil {
		t.Fatalf("StateAt(epoch): %v", err)
	}

	after := time.J2000.AddDays(ioPeriodDays)

	closed, _, err := tabulated.StateAt(after)
	if err != nil {
		t.Fatalf("StateAt(epoch + P): %v", err)
	}

	if rel := closed.Sub(start).Norm() / start.Norm(); rel > 1e-12 {
		t.Errorf("one supplied period left the orbit a relative %.3g from where it began; "+
			"the mean anomaly is not advancing at the supplied rate", rel)
	}

	// The same instant under the derived mean motion must land elsewhere:
	// 0.4% of a revolution is 1.4°, which on this orbit is thousands of km.
	open, _, err := derived.StateAt(after)
	if err != nil {
		t.Fatalf("StateAt(derived, epoch + P): %v", err)
	}

	if rel := open.Sub(start).Norm() / start.Norm(); rel < 1e-3 {
		t.Errorf("the derived period closed the orbit too (relative %.3g); WithPeriod "+
			"cannot be shown to have changed anything", rel)
	}
}

// TestPeriodZeroRestoresTheDerivedMeanMotion pins the sentence in
// [kepler.Elements.WithPeriod]'s doc comment that says so. A zero that
// silently meant "a period of zero days" would divide by zero rather than
// fall back, and nothing else in the suite would notice.
func TestPeriodZeroRestoresTheDerivedMeanMotion(t *testing.T) {
	jupiter, _ := kepler.CentralBodyFor(core.Jupiter)

	derived := circular(t, jupiter, 421_800)
	cleared := derived.WithPeriod(3).WithPeriod(0)

	if p := cleared.Period(); p != 0 {
		t.Errorf("Period() = %v after WithPeriod(0), want 0", p)
	}

	at := time.J2000.AddDays(9.5)

	want, _, err := derived.StateAt(at)
	if err != nil {
		t.Fatalf("StateAt(derived): %v", err)
	}

	got, _, err := cleared.StateAt(at)
	if err != nil {
		t.Fatalf("StateAt(cleared): %v", err)
	}

	if got != want {
		t.Errorf("WithPeriod(0) propagated to %v, derived elements to %v", got, want)
	}
}

// TestSecularPrecessionTurnsTheApsisBackwards pins the sign, which is the
// one thing about [kepler.SecularPrecession] that cannot be got right by
// accident — its own doc comment records that the other sign is eighteen
// times worse against Horizons.
//
// The check reconstructs the expected orientation independently: elements
// whose argument of periapsis is hand-advanced by -2*pi*dt/(P*365.25) and
// carry no precession must propagate to the same state as elements that
// carry the precession and let it do the advancing.
func TestSecularPrecessionTurnsTheApsisBackwards(t *testing.T) {
	jupiter, _ := kepler.CentralBodyFor(core.Jupiter)

	// Io's apsis period, in Julian years, from the same JPL table as its
	// elements. An eccentric, inclined orbit, so that moving the apsis moves
	// the position: on a circular equatorial orbit it would not.
	const (
		apsisYears = 0.0069
		argpDeg    = 49.1
	)

	base, err := kepler.NewElements(time.J2000, kmToAU(421_800), 0.004,
		angle.Deg(2), angle.Deg(0), angle.Deg(argpDeg), angle.Deg(330.9))
	if err != nil {
		t.Fatalf("NewElements: %v", err)
	}

	base = base.WithCentralBody(jupiter)

	if p := base.SecularPrecession(); p.ApsisPeriod != 0 {
		t.Errorf("SecularPrecession() = %+v on elements that never set one, want zero", p)
	}

	prec := kepler.SecularPrecession{ApsisPeriod: apsisYears}

	drifting := base.WithSecularPrecession(prec)
	if got := drifting.SecularPrecession(); got != prec {
		t.Errorf("SecularPrecession() = %+v, want %+v", got, prec)
	}

	const dtDays = 10

	at := time.J2000.AddDays(dtDays)

	// The documented rate, rebuilt here rather than read from the package.
	const daysPerJulianYear = 365.25

	turned := angle.Deg(argpDeg).Radians() - 2*math.Pi*dtDays/(apsisYears*daysPerJulianYear)

	hand, err := kepler.NewElements(time.J2000, kmToAU(421_800), 0.004,
		angle.Deg(2), angle.Deg(0), angle.Rad(turned), angle.Deg(330.9))
	if err != nil {
		t.Fatalf("NewElements(hand): %v", err)
	}

	want, _, err := hand.WithCentralBody(jupiter).StateAt(at)
	if err != nil {
		t.Fatalf("StateAt(hand): %v", err)
	}

	got, _, err := drifting.StateAt(at)
	if err != nil {
		t.Fatalf("StateAt(drifting): %v", err)
	}

	if sep := got.Sub(want).Norm() * auMeters / 1e3; sep > 1e-6 {
		t.Errorf("precession put the satellite %.3g km from a hand-advanced apsis; "+
			"the rate or its sign disagrees with the documented one", sep)
	}

	// And it is not a no-op: over ten days Io's apsis turns 7.4 degrees, so
	// the fixed ellipse must land somewhere else entirely.
	fixed, _, err := base.StateAt(at)
	if err != nil {
		t.Fatalf("StateAt(base): %v", err)
	}

	if sep := fixed.Sub(got).Norm() * auMeters / 1e3; sep < 100 {
		t.Errorf("precession moved the satellite only %.3g km in ten days; "+
			"it cannot be shown to have been applied", sep)
	}
}

// TestSecularPrecessionZeroIsNoDrift keeps the zero value meaning "none",
// so that an Elements built from a table with no apsis column propagates
// exactly as one that never mentioned precession.
func TestSecularPrecessionZeroIsNoDrift(t *testing.T) {
	jupiter, _ := kepler.CentralBodyFor(core.Jupiter)

	base := circular(t, jupiter, 421_800)
	zeroed := base.WithSecularPrecession(kepler.SecularPrecession{})

	at := time.J2000.AddDays(40)

	want, _, err := base.StateAt(at)
	if err != nil {
		t.Fatalf("StateAt(base): %v", err)
	}

	got, _, err := zeroed.StateAt(at)
	if err != nil {
		t.Fatalf("StateAt(zeroed): %v", err)
	}

	if got != want {
		t.Errorf("a zero SecularPrecession propagated to %v, no precession to %v", got, want)
	}
}

// TestSatelliteFailureNamesItsParent covers the failure this whole change
// exists because of: a satellite whose parent cannot be resolved.
//
// Before Pluto was registered on the base, a Charon orbit failed *at
// Pluto*, and the error said only that some body was unsupported. Composing
// through the parent means every satellite inherits its parent's failures,
// so the error has to say which body was being asked for — otherwise the
// next occurrence is as hard to read as that one was.
//
// mockBaseProvider is reused from provider_test.go: it fails for every id,
// which is exactly a base that cannot answer for the parent.
func TestSatelliteFailureNamesItsParent(t *testing.T) {
	jupiter, ok := kepler.CentralBodyFor(core.Jupiter)
	if !ok {
		t.Fatal("no central body for Jupiter")
	}

	const ioID core.ID = 501

	p := kepler.New(kepler.WithBase(&mockBaseProvider{wantErr: errMockBase}))
	if err := p.Register(ioID, circular(t, jupiter, 421_800)); err != nil {
		t.Fatalf("Register: %v", err)
	}

	_, err := p.State(ioID, time.J2000)
	if err == nil {
		t.Fatal("State succeeded with a base that cannot answer for the parent")
	}

	if !errors.Is(err, errMockBase) {
		t.Errorf("error does not wrap the base's own: %v", err)
	}

	// The satellite's own id is not the useful one here — the caller asked
	// for it and knows it. The parent is what they cannot see.
	if msg := err.Error(); !strings.Contains(msg, "central body") ||
		!strings.Contains(msg, core.Jupiter.String()) {
		t.Errorf("error %q names neither the role nor the parent body", msg)
	}
}
