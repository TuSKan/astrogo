package ephemeris_test

import (
	"errors"
	"math"
	"slices"
	"strings"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// errProviderFailed is what the scripted provider below returns, so the tests
// can assert the wrapping preserved it rather than matching on a message.
var errProviderFailed = errors.New("provider refused")

// scriptedProvider answers with a fixed geometry and fails for exactly one
// (body, epoch) combination.
//
// One combination at a time is the point. [eph.ApparentState] makes four
// distinct provider calls — the target now, the target retarded, the Sun now,
// the Sun retarded — and each has its own error return. A provider that failed
// for everything would prove that one of them works and say nothing about the
// other three, which is how an unreachable error path stays unreachable.
type scriptedProvider struct {
	target  vector.Vec3
	sun     vector.Vec3
	obsTime time.Time

	failBody    eph.ID
	failAtObs   bool // fail at obsTime rather than at the retarded epoch
	failEnabled bool
}

func (p *scriptedProvider) State(id eph.ID, t time.Time) (eph.State, error) {
	if p.failEnabled && id == p.failBody && t.Equal(p.obsTime) == p.failAtObs {
		return eph.State{}, errProviderFailed
	}

	if id == eph.Sun {
		return eph.State{Pos: p.sun, Vel: vector.Zero()}, nil
	}

	return eph.State{Pos: p.target, Vel: vector.Zero()}, nil
}

func (p *scriptedProvider) Close() error { return nil }

// TestApparentStateReportsWhichFetchFailed covers the four error returns on
// the apparent-place path, one at a time.
//
// # Why each one separately
//
// These are the paths #263 added and left untested, and they are the class
// this repository has been bitten by before: a failure that is
// indistinguishable from a legitimate answer. Every one of them returns
// State{} on the way out, and State{} is a real value — a body at the
// geocentre, at rest. A caller that ignored the error would get a plausible
// zero rather than a crash, which is why the assertion here is on all three of
// the error being returned, the error still carrying its cause, and the state
// being untouched.
//
// The messages are also asserted, loosely, because they are the only thing
// that distinguishes four failures with identical types: "which fetch failed"
// is the entire diagnostic value of having four separate returns instead of
// one.
func TestApparentStateReportsWhichFetchFailed(t *testing.T) {
	t.Parallel()

	obsTime := time.Date(2026, time.June, 21, 0, 0, 0, 0, time.LocationUTC)

	cases := []struct {
		name      string
		failBody  eph.ID
		failAtObs bool
		wants     string
	}{
		{"target at the observing epoch", eph.Mars, true, "apparent state"},
		{"target at the retarded epoch", eph.Mars, false, "retarded"},
		{"Sun at the observing epoch", eph.Sun, true, "sun for light deflection"},
		{"Sun at the retarded epoch", eph.Sun, false, "retarded sun for light deflection"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := &scriptedProvider{
				target:      vector.V3(0.5, 0.3, 0.1),
				sun:         vector.V3(-0.9, 0.4, 0.0),
				obsTime:     obsTime,
				failBody:    tc.failBody,
				failAtObs:   tc.failAtObs,
				failEnabled: true,
			}

			st, err := eph.ApparentState(p, eph.Mars, obsTime)
			if err == nil {
				t.Fatalf("provider failed for %s and ApparentState returned no error, "+
					"state %+v.\n"+
					"  A silent State{} here is a body at the geocentre at rest, which is a "+
					"legitimate-looking answer rather than a reported failure.", tc.name, st)
			}

			if !errors.Is(err, errProviderFailed) {
				t.Errorf("error does not wrap the provider's own: %v.\n"+
					"  Use %%w, so a caller can tell a provider outage from a bad argument.", err)
			}

			if !strings.Contains(err.Error(), tc.wants) {
				t.Errorf("error %q does not say %q.\n"+
					"  Four fetches share one path and one error type; the message is the only "+
					"thing that says which of them failed.", err, tc.wants)
			}

			if st != (eph.State{}) {
				t.Errorf("state on failure is %+v, want the zero State.\n"+
					"  Returning a partially filled state alongside an error invites a caller "+
					"to use it.", st)
			}
		})
	}
}

// TestApparentStateSurvivesDegenerateDeflectionGeometry covers the guard that
// keeps three impossible geometries from becoming NaN.
//
// Light deflection needs three directions, and each of them comes from a
// subtraction that can cancel: a target at the geocentre has no direction from
// the observer, a target at the Sun's own place has none from the Sun, and a
// Sun at the geocentre has none to the observer. Normalising any of those
// yields NaN, and NaN propagates silently all the way to an altitude and an
// azimuth that are simply absent from a plot.
//
// None of the three is physical. They are reachable anyway — a synthetic
// provider in a caller's own test, a body whose ephemeris briefly returns the
// origin — and the guard's contract is that the position comes back undeflected
// rather than not at all.
func TestApparentStateSurvivesDegenerateDeflectionGeometry(t *testing.T) {
	t.Parallel()

	obsTime := time.Date(2026, time.June, 21, 0, 0, 0, 0, time.LocationUTC)

	cases := []struct {
		name     string
		pos, sun vector.Vec3
	}{
		{
			"target at the geocentre",
			vector.Zero(),
			vector.V3(-0.9, 0.4, 0.0),
		},
		{
			"target at the Sun's own place",
			vector.V3(-0.9, 0.4, 0.0),
			vector.V3(-0.9, 0.4, 0.0),
		},
		{
			"Sun at the geocentre",
			vector.V3(0.5, 0.3, 0.1),
			vector.Zero(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := &staticProvider{pos: tc.pos, sun: tc.sun}

			st, err := eph.ApparentState(p, eph.Mars, obsTime)
			if err != nil {
				t.Fatalf("ApparentState: %v", err)
			}

			for axis, v := range map[string]float64{
				"X": st.Pos.X, "Y": st.Pos.Y, "Z": st.Pos.Z,
			} {
				if math.IsNaN(v) || math.IsInf(v, 0) {
					t.Fatalf("position %s is %v, from %+v.\n"+
						"  A degenerate deflection geometry has to leave the place alone; NaN "+
						"reaches an altitude and an azimuth and disappears from a plot rather "+
						"than failing.", axis, v, st.Pos)
				}
			}

			if st.Pos != tc.pos {
				t.Errorf("position is %+v, want %+v unchanged.\n"+
					"  With no usable geometry the deflection is not defined, so the guard "+
					"returns the input rather than an approximation of it.", st.Pos, tc.pos)
			}
		})
	}
}

// movingSunProvider puts the Sun on an eccentric geocentric orbit and
// remembers every position it handed out.
//
// The motion makes the light-time iteration do real work instead of converging
// on a constant range, and the record is what lets a test assert an exact
// value without replicating that iteration to find out which epoch it settled
// on.
type movingSunProvider struct {
	returned []vector.Vec3
	epoch    time.Time
}

func (p *movingSunProvider) State(_ eph.ID, t time.Time) (eph.State, error) {
	const (
		period = 365.25
		// Eccentric rather than circular, so the range changes and the
		// light-time loop has something to iterate on. A circular orbit
		// converges on the first pass because the range never moves, which
		// makes this a test of a constant.
		eccentricity = 0.2
	)

	w := 2 * math.Pi / period
	d := t.JD() - p.epoch.JD()
	s, c := math.Sincos(w * d)

	radius := 1.0 + eccentricity*math.Sin(3*w*d)

	pos := vector.V3(radius*c, radius*s, 0)
	p.returned = append(p.returned, pos)

	return eph.State{Pos: pos, Vel: vector.V3(-radius*w*s, radius*w*c, 0)}, nil
}

func (p *movingSunProvider) Close() error { return nil }

// TestApparentStateDoesNotDeflectTheSun pins the one body that must not be
// bent by its own gravity.
//
// deflectBySun finds the Sun-to-source direction by subtracting the Sun's
// geocentric position from the target's. For the Sun those are the same body,
// so there is no direction to find, and the formula would be normalising
// whatever is left of two nearly identical vectors.
//
// # What this asserts, and what it does not
//
// It asserts the contract: the Sun's apparent place is light time and nothing
// else. That is what a caller depends on, and it holds however the code
// arranges to deliver it.
//
// It does *not* discriminate between the two things in deflectBySun that
// deliver it, and the attempt is recorded here so nobody repeats it. Deleting
// the `target == core.Sun` early return does not fail this test, with a static
// Sun or a moving one, because the leftover is not merely small — once the
// light-time iteration reaches a fixed point, the epoch it settles on is the
// epoch the position was taken at, so the subtraction is *exactly* zero and
// the degenerate-geometry guard above catches the case. Measured with an
// eccentric Sun and a 1e-11-day tolerance: |Sun − Sun| came out at zero to
// better than 1e-18 au.
//
// So the early return is a fast path and a statement of intent, not the thing
// standing between the Sun and a spurious deflection. Writing this comment the
// other way round would have been the easy mistake.
//
// The Sun moves here anyway, on an eccentric orbit, because that exercises a
// real light-time iteration rather than a constant. The assertion is
// bit-for-bit against the positions the provider actually returned, so it needs
// no tolerance and no second implementation of the loop: an apparent place that
// no provider call produced is a deflected one.
func TestApparentStateDoesNotDeflectTheSun(t *testing.T) {
	t.Parallel()

	epoch := time.Date(2026, time.June, 21, 0, 0, 0, 0, time.LocationUTC)
	p := &movingSunProvider{epoch: epoch}

	st, err := eph.ApparentState(p, eph.Sun, epoch)
	if err != nil {
		t.Fatalf("ApparentState: %v", err)
	}

	if slices.Contains(p.returned, st.Pos) {
		return
	}

	t.Errorf("the Sun's apparent position is %+v, which is none of the %d positions the "+
		"provider returned (first %+v, last %+v).\n"+
		"  Nothing may deflect the Sun by the Sun: the Sun-to-source direction there is the "+
		"residue of the light-time iteration, not a direction, and normalising it bends the "+
		"Sun by milliarcseconds towards nowhere in particular.",
		st.Pos, len(p.returned), p.returned[0], p.returned[len(p.returned)-1])
}

// TestApparentStateDeflectsAnOrdinaryGeometry is the positive control for the
// three tests above.
//
// Each of them passes if the position comes back unchanged, so all three would
// keep passing with the deflection deleted. This one fails in that case, which
// is what makes the others mean what they say.
func TestApparentStateDeflectsAnOrdinaryGeometry(t *testing.T) {
	t.Parallel()

	obsTime := time.Date(2026, time.June, 21, 0, 0, 0, 0, time.LocationUTC)

	// A target roughly 20 degrees from the Sun, where the deflection is a few
	// hundredths of an arcsecond — small, and thousands of times larger than
	// the float64 noise this compares against.
	pos := vector.V3(-0.75, 0.62, 0.05)
	sun := vector.V3(-0.9, 0.4, 0.0)

	p := &staticProvider{pos: pos, sun: sun}

	st, err := eph.ApparentState(p, eph.Mars, obsTime)
	if err != nil {
		t.Fatalf("ApparentState: %v", err)
	}

	if st.Pos == pos {
		t.Fatal("the apparent position is bit-for-bit the geometric one.\n" +
			"  Deflection is not being applied, and the degenerate-geometry and Sun tests " +
			"in this file would pass anyway — they assert that the position is unchanged.")
	}

	// Range is set by light time and must survive the direction correction:
	// Ld returns a vector that is not quite unit, and carrying that through
	// would quietly rescale every distance.
	if got, want := st.Pos.Norm(), pos.Norm(); math.Abs(got-want) > 1e-15*want {
		t.Errorf("range changed from %.15g to %.15g au.\n"+
			"  Deflection bends a ray; it does not move the target along it.", want, got)
	}
}
