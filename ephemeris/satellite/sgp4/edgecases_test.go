package sgp4_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/ephemeris/satellite/sgp4"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// The paths Vallado's verification suite does not reach.
//
// Every element set in that suite is a real object, and real objects do not sit
// exactly on a singularity or carry a drag term that drives them into the
// ground inside a day. So the reference proves the model right where it is
// well-behaved and says nothing about the guards — which is where an
// implementation is most likely to differ, because the guards are the parts
// that are there to be stepped on rather than executed.

// leo returns a well-behaved low Earth orbit that each case below changes one
// thing about.
func leo() sgp4.Elements {
	return sgp4.Elements{
		NORAD:        1,
		Epoch:        time.Date(2024, time.January, 1, 0, 0, 0, 0, time.LocationUTC),
		Inclination:  angle.Deg(51.6),
		RAAN:         angle.Deg(247.0),
		ArgPerigee:   angle.Deg(130.0),
		MeanAnomaly:  angle.Deg(325.0),
		Eccentricity: 0.0006,
		MeanMotion:   15.7,
		BStar:        1e-4,
	}
}

// TestTheModelsErrorPathsAreReachable exercises each error the propagation can
// raise, with an input that raises it.
//
// Two of these — a non-positive mean motion and a sub-surface position — are
// unreachable from Vallado's suite, because real objects do not sit on a
// singularity or carry a drag term that puts them underground inside a day. So
// they are constructed.
//
// # Why this scans a range instead of naming a time
//
// It named one at first, and macOS failed on it. An input degenerate enough to
// drive the model into an error is degenerate enough to reach SEVERAL of them,
// and which one arrives first at a given instant comes down to the last bits:
// at tsince 5600 the mean motion goes non-positive first on amd64 and the
// semi-latus rectum goes negative first on arm64, where fused multiply-add is
// permitted. Both are correct. Pinning the instant was asserting a race.
//
// So the claim is the one actually worth making — this input reaches this error
// somewhere in this span — and the test reports where it found it.
func TestTheModelsErrorPathsAreReachable(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		mutate   func(*sgp4.Elements)
		from, to float64
		wantErr  error
		state    bool // whether a usable state comes back with the error
	}{
		{
			// A drag term four thousand times a real one drives the orbit into
			// the ground within hours.
			name:    "decayed below the surface",
			mutate:  func(e *sgp4.Elements) { e.BStar = 0.5 },
			from:    0,
			to:      2000,
			wantErr: sgp4.ErrDecayed,
			state:   true,
		},
		{
			name:    "mean eccentricity leaves range under drag",
			mutate:  func(e *sgp4.Elements) { e.BStar = 0.5 },
			from:    0,
			to:      5000,
			wantErr: sgp4.ErrEccentricity,
		},
		{
			name: "semi-latus rectum goes negative",
			mutate: func(e *sgp4.Elements) {
				e.Eccentricity = 0.9999999 // the most a TLE field can express
				e.MeanMotion = 2.0
			},
			from:    0,
			to:      1000,
			wantErr: sgp4.ErrSemiLatusRectum,
		},
		{
			name: "mean motion goes non-positive",
			mutate: func(e *sgp4.Elements) {
				e.Eccentricity = 0.9999999
				e.MeanMotion = 2.0
			},
			from:    0,
			to:      40000,
			wantErr: sgp4.ErrMeanMotion,
		},
	} {
		el := leo()
		tc.mutate(&el)

		p, err := sgp4.New(el)
		if err != nil {
			t.Errorf("%s: New: %v", tc.name, err)

			continue
		}

		var (
			found bool
			at    float64
			pos   vector.Vec3
			vel   vector.Vec3
		)

		for ts := tc.from; ts <= tc.to && !found; ts += 25 {
			pos, vel, err = p.At(ts)
			if errors.Is(err, tc.wantErr) {
				found = true
				at = ts
			}
		}

		if !found {
			t.Errorf("%s: %v was not raised anywhere in tsince [%g, %g]",
				tc.name, tc.wantErr, tc.from, tc.to)

			continue
		}

		t.Logf("%s: %v first raised at tsince %g", tc.name, tc.wantErr, at)

		// The contract stated on Propagator.At: two of these come with the
		// state that was computed, and every other one leaves it zero. A caller
		// who inspects the state after an error relies on exactly this.
		zero := pos == (vector.Vec3{}) && vel == (vector.Vec3{})

		if tc.state && zero {
			t.Errorf("%s: %v came back with a zero state; it is documented as carrying one",
				tc.name, tc.wantErr)
		}

		if !tc.state && !zero {
			t.Errorf("%s: %v came back with a populated state; only ErrDecayed and "+
				"ErrKeplerNotConverged are documented as doing that", tc.name, tc.wantErr)
		}
	}
}

// TestKeplerNonConvergenceIsNotReachableFromAnElementSet records a negative
// result, because the alternative is a sentinel nobody can say anything about.
//
// ErrKeplerNotConverged exists because the reference's iteration can give up:
// ten passes, a correction clamped to ±0.95 radians, and no report when it ends
// without converging. Vallado's own note reads "the following iteration needs
// better limits on corrections".
//
// It has not been possible to construct an element set that reaches it. This
// tries the shapes that should be worst — the largest eccentricity a TLE can
// express, at several mean motions, across a wide span of times and in both
// directions — and the iteration converges every time. So the sentinel is
// documented as covering inputs this package has not found, which is a weaker
// and more honest claim than the one first written here, and this test is what
// makes the claim checkable: if a change ever does make it fire, this fails and
// says so.
func TestKeplerNonConvergenceIsNotReachableFromAnElementSet(t *testing.T) {
	t.Parallel()

	for _, n := range []float64{0.5, 1.0027, 2.0, 8.0, 16.0} {
		el := leo()
		el.Eccentricity = 0.9999999
		el.MeanMotion = n

		// Refusing the shape would leave nothing tried, and the negative result
		// above would then rest on nothing. New accepts all five today.
		p, err := sgp4.New(el)
		if err != nil {
			t.Fatalf("mean motion %g at e = %g: New: %v", n, el.Eccentricity, err)
		}

		for ts := -50000.0; ts <= 50000.0; ts += 250 {
			if _, _, err := p.At(ts); errors.Is(err, sgp4.ErrKeplerNotConverged) {
				t.Errorf("mean motion %g reached ErrKeplerNotConverged at tsince %g — the "+
					"sentinel's doc comment says no input has been found that does, and "+
					"should be updated to name this one", n, ts)

				return
			}
		}
	}
}

// TestInclinationSingularityIsGuarded drives the model onto the one geometric
// singularity it has to handle: an exactly retrograde orbit, where 1 + cos(i)
// is zero and two of the long-period coefficients divide by it.
//
// The reference substitutes 1.5e-12 for the divisor there. Vallado's own note
// explains the value: the original check used 1 + cos(pi − 1e-9) and compared
// it against 1.5e-12, so the threshold was changed to match. Nothing in his
// verification suite has an inclination anywhere near 180 degrees.
func TestInclinationSingularityIsGuarded(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		incl angle.Angle
		n    float64
	}{
		{"exactly retrograde, near Earth", angle.Rad(math.Pi), 15.7},
		{"exactly retrograde, deep space", angle.Rad(math.Pi), 1.0027},
		{"one nanoradian off retrograde", angle.Rad(math.Pi - 1e-9), 15.7},
		{"exactly equatorial", 0, 15.7},
		{"equatorial, deep space", 0, 1.0027},
	} {
		el := leo()
		el.Inclination = tc.incl
		el.MeanMotion = tc.n

		p, err := sgp4.New(el)
		if err != nil {
			t.Errorf("%s: New: %v", tc.name, err)

			continue
		}

		for _, ts := range []float64{0, 100, 1000, -1000} {
			pos, vel, err := p.At(ts)
			if err != nil {
				t.Errorf("%s at tsince %g: %v", tc.name, ts, err)

				continue
			}

			for _, v := range []float64{pos.X, pos.Y, pos.Z, vel.X, vel.Y, vel.Z} {
				if math.IsNaN(v) || math.IsInf(v, 0) {
					t.Errorf("%s at tsince %g produced %v — the singularity guard did not hold",
						tc.name, ts, v)

					break
				}
			}

			// A finite answer is not enough: it also has to be an orbit.
			if r := pos.Norm(); r < 6000 || r > 1e7 {
				t.Errorf("%s at tsince %g is %g km from the center of the Earth",
					tc.name, ts, r)
			}
		}
	}
}

// TestAFSPCModeIsADifferentAnswer covers the operational convention the
// reference states were not generated with, in both directions.
//
// The two modes differ in exactly two places, and both are deep space: sidereal
// time at epoch, which reaches the model only through dscom/dsinit/dspace, and
// the node normalization inside dpper's Lyddane branch. So the assertion is not
// simply "they differ" — below the 225-minute threshold they must be
// bit-identical, and above it they must differ by the small amount a convention
// produces rather than the large amount a bug produces.
//
// The near-Earth half is the one worth having. It was written expecting a
// difference, it found none, and the reason turned out to be a property of the
// model that Mode's doc comment did not state until this test was written.
func TestAFSPCModeIsADifferentAnswer(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		n         float64
		incl      angle.Angle
		wantDiffs bool
	}{
		{"near Earth", 15.7, angle.Deg(51.6), false},
		{"deep space", 1.0027, angle.Deg(51.6), true},
		// Low inclination takes dpper's Lyddane branch, which is the second of
		// the two places the modes part company.
		{"deep space, near-equatorial", 1.0027, angle.Deg(0.05), true},
	} {
		el := leo()
		el.MeanMotion = tc.n
		el.Inclination = tc.incl

		improved, err := sgp4.New(el, sgp4.WithMode(sgp4.ModeImproved))
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}

		afspc, err := sgp4.New(el, sgp4.WithMode(sgp4.ModeAFSPC))
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}

		var maxDiff float64

		for ts := -2880.0; ts <= 2880.0; ts += 60 {
			a, _, aerr := improved.At(ts)
			b, _, berr := afspc.At(ts)

			if aerr != nil || berr != nil {
				t.Errorf("%s at tsince %g: improved %v, afspc %v", tc.name, ts, aerr, berr)

				continue
			}

			for _, v := range []float64{b.X, b.Y, b.Z} {
				if math.IsNaN(v) || math.IsInf(v, 0) {
					t.Fatalf("%s: AFSPC mode produced %v at tsince %g", tc.name, v, ts)
				}
			}

			if d := a.Sub(b).Norm(); d > maxDiff {
				maxDiff = d
			}
		}

		switch {
		case !tc.wantDiffs && maxDiff != 0:
			t.Errorf("%s: the two modes differ by %g km. Below the deep-space threshold "+
				"they read no quantity that distinguishes them, so they must be "+
				"bit-identical", tc.name, maxDiff)
		case tc.wantDiffs && maxDiff == 0:
			t.Errorf("%s: the two modes are identical, so WithMode is not reaching the "+
				"deep-space code — it should, through the sidereal time at epoch", tc.name)
		case maxDiff > 100:
			t.Errorf("%s: the two modes differ by %g km. A convention difference is meters "+
				"to a few km; this is a bug", tc.name, maxDiff)
		default:
			t.Logf("%s: the two modes differ by up to %.4g km", tc.name, maxDiff)
		}
	}
}

// TestWithGravityReachesTheModel is the option whose default was worth 93x the
// position error when it was wrong, so it is worth knowing the switch works.
func TestWithGravityReachesTheModel(t *testing.T) {
	t.Parallel()

	el := leo()

	byModel := make(map[sgp4.Gravity]float64)

	for _, g := range []sgp4.Gravity{sgp4.WGS72, sgp4.WGS84, sgp4.WGS72Old} {
		p, err := sgp4.New(el, sgp4.WithGravity(g))
		if err != nil {
			t.Fatalf("%v: %v", g, err)
		}

		pos, _, err := p.At(1440)
		if err != nil {
			t.Fatalf("%v: %v", g, err)
		}

		byModel[g] = pos.X
	}

	if byModel[sgp4.WGS72] == byModel[sgp4.WGS84] {
		t.Error("WGS-72 and WGS-84 produced an identical position, so WithGravity is not " +
			"reaching the model")
	}

	// WGS72Old differs from WGS72 only in mu and a hardcoded xke, so the gap is
	// small — and it must not be zero, or the third model is a duplicate of the
	// first and TestWGS72OldKeepsItsHardcodedXKE is guarding nothing observable.
	if byModel[sgp4.WGS72] == byModel[sgp4.WGS72Old] {
		t.Error("WGS-72 and WGS-72-old produced an identical position; the hardcoded xke " +
			"that distinguishes them is not reaching the model")
	}

	// The default must be WGS-72, since that is the model TLEs are fitted with.
	def, err := sgp4.New(el)
	if err != nil {
		t.Fatal(err)
	}

	pos, _, err := def.At(1440)
	if err != nil {
		t.Fatal(err)
	}

	if pos.X != byModel[sgp4.WGS72] {
		t.Errorf("the default gravity model is not WGS-72")
	}
}

// TestNewRefusesAnUnknownOption checks that a Gravity or Mode outside the
// declared constants is refused at construction rather than silently falling
// through to a default. gravity_test.go records that constantsFor falls back to
// the WGS-72 row; this is the half that stops a caller ever seeing it.
func TestNewRefusesAnUnknownOption(t *testing.T) {
	t.Parallel()

	if _, err := sgp4.New(leo(), sgp4.WithGravity(sgp4.Gravity(42))); !errors.Is(err, sgp4.ErrElements) {
		t.Errorf("New accepted Gravity(42): %v", err)
	}

	if _, err := sgp4.New(leo(), sgp4.WithMode(sgp4.Mode(42))); !errors.Is(err, sgp4.ErrElements) {
		t.Errorf("New accepted Mode(42): %v", err)
	}

	if got := sgp4.ModeImproved.String(); got != "improved" {
		t.Errorf("ModeImproved.String() = %q", got)
	}

	if got := sgp4.ModeAFSPC.String(); got != "AFSPC" {
		t.Errorf("ModeAFSPC.String() = %q", got)
	}

	if got := sgp4.Mode(42).String(); got != "Mode(42)" {
		t.Errorf("Mode(42).String() = %q, want %q", got, "Mode(42)")
	}
}

// TestAtTimeAgreesWithAt covers the absolute-time entry point, and the scale
// conversion inside it.
//
// The conversion is the part worth testing rather than the arithmetic: SGP4 is
// defined against UTC, and a TT instant taken at face value lands 69.184 s late
// — 530 km for a low Earth orbit. time.Time is scale-aware precisely so that
// cannot be left to the caller, and AtTime is where astrogo makes that good.
func TestAtTimeAgreesWithAt(t *testing.T) {
	t.Parallel()

	el := leo()

	p, err := sgp4.New(el)
	if err != nil {
		t.Fatal(err)
	}

	for _, minutes := range []float64{0, 1, 37.5, 1440, -720} {
		want, _, werr := p.At(minutes)
		if werr != nil {
			t.Fatalf("At(%g): %v", minutes, werr)
		}

		at := el.Epoch.Add(unit.Days(minutes / 1440.0))

		got, _, gerr := p.AtTime(at)
		if gerr != nil {
			t.Fatalf("AtTime(%s): %v", at, gerr)
		}

		// Not exact equality: AtTime forms tsince from the UTC labels'
		// whole seconds and nanoseconds, which is a different route to the
		// same number, and the last bits of that route move with the platform. A millimeter is two
		// orders below the package's own contract and four below anything a
		// caller could notice, so it bounds "the same computation" without
		// asserting bit-identity across architectures.
		if d := want.Sub(got).Norm(); d > 1e-6 {
			t.Errorf("At(%g) and AtTime of the same instant differ by %g km", minutes, d)
		}
	}

	// The scale conversion. The same instant labeled TT is 69.184 s later than
	// the UTC label, and AtTime must resolve that rather than take the label.
	utc := el.Epoch.Add(unit.Days(1))

	fromUTC, _, err := p.AtTime(utc)
	if err != nil {
		t.Fatal(err)
	}

	fromTT, _, err := p.AtTime(utc.TT())
	if err != nil {
		t.Fatal(err)
	}

	if d := fromUTC.Sub(fromTT).Norm(); d > 1e-6 {
		t.Errorf("the same instant given as UTC and as TT produced positions %g km apart; "+
			"AtTime is reading the label instead of converting the scale", d)
	}
}

// TestAccessorsDescribeTheOrbit covers the reporting surface, which is the part
// a caller reads before deciding whether to trust a propagation at all.
func TestAccessorsDescribeTheOrbit(t *testing.T) {
	t.Parallel()

	el := leo()

	p, err := sgp4.New(el)
	if err != nil {
		t.Fatal(err)
	}

	if got := p.Elements(); got != el {
		t.Error("Elements() did not return the element set the propagator was built from")
	}

	if p.DeepSpace() {
		t.Error("a 15.7 rev/day orbit is not deep space")
	}

	if p.SimplifiedDrag() {
		t.Error("a 400 km orbit does not take the simplified drag branch")
	}

	// A 15.7 rev/day near-circular orbit sits around 400 km. Loose bounds:
	// the point is that the two are the right way round and in the right
	// neighbourhood, not to re-derive the orbit here.
	perigee, apogee := p.PerigeeAltitude(), p.ApogeeAltitude()

	if perigee > apogee {
		t.Errorf("perigee %g km is above apogee %g km", perigee, apogee)
	}

	if perigee < 300 || apogee > 500 {
		t.Errorf("perigee %g km and apogee %g km are not a 400 km orbit", perigee, apogee)
	}

	// Deep space, for the other side of the branch.
	geo := leo()
	geo.MeanMotion = 1.0027

	g, err := sgp4.New(geo)
	if err != nil {
		t.Fatal(err)
	}

	if !g.DeepSpace() {
		t.Error("a one-revolution-per-day orbit is deep space")
	}

	if !g.SimplifiedDrag() {
		t.Error("SDP4 always takes the simplified drag branch, whatever the perigee")
	}
}

// TestAtTimeCountsLabelsOnALeapSecondDay pins SGP4's clock on the 27 days that
// end in a leap second.
//
// SGP4 counts UTC the way its reference implementation's jday does, every day
// 1440 minutes, so noon is 720 minutes after the midnight before it on any day
// at all. A UTC Julian Date's fraction on such a day is of 86401 seconds,
// following SOFA (#144), so subtracting two of them — which AtTime used to do —
// puts noon at 719.99167 minutes: half a second short, 3.8 km of low-Earth-
// orbit track.
func TestAtTimeCountsLabelsOnALeapSecondDay(t *testing.T) {
	t.Parallel()

	el := leo()
	el.Epoch = time.Date(2016, time.December, 31, 0, 0, 0, 0, time.LocationUTC)

	p, err := sgp4.New(el)
	if err != nil {
		t.Fatal(err)
	}

	want, _, err := p.At(720)
	if err != nil {
		t.Fatalf("At(720): %v", err)
	}

	got, _, err := p.AtTime(time.Date(2016, time.December, 31, 12, 0, 0, 0, time.LocationUTC))
	if err != nil {
		t.Fatalf("AtTime(noon): %v", err)
	}

	if d := want.Sub(got).Norm(); d > 1e-6 {
		t.Errorf("noon on the leap-second day is %g km from At(720); SGP4 counts that as "+
			"exactly 720 minutes", d)
	}
}
