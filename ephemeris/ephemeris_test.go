package ephemeris

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// This package's arithmetic was well covered and its front door was not:
// every constructor and every option sat at 0%. That is the worst place for
// a gap. A broken option does not fail where it is written — it fails later,
// somewhere that has no idea an option was involved, which is the shape of
// most of the defects this repository has had to chase.
//
// Everything below runs offline. The kernel-backed branch of NewProvider
// needs a download and is covered by ephemeris/jpl's own network suites; what
// is checked here is the dispatch around it, which is where the untested
// surface actually was.

// stubProvider answers one body with a fixed state and refuses everything
// else, so a test can tell whether a call reached the base provider.
type stubProvider struct {
	id     core.ID
	pos    [3]float64
	calls  int
	closed bool
}

func (s *stubProvider) State(id core.ID, _ time.Time) (State, error) {
	s.calls++

	if id != s.id {
		return State{}, ErrUnsupportedBody
	}

	return State{
		Pos:    vec(s.pos),
		Frame:  core.FrameICRS,
		Center: core.CenterGeocentre,
	}, nil
}

func (s *stubProvider) Close() error {
	s.closed = true

	return nil
}

func vec(a [3]float64) vector.Vec3 { return vector.Vec3{X: a[0], Y: a[1], Z: a[2]} }

func TestNewElementsRejectsWhatTwoBodyCannotRepresent(t *testing.T) {
	epoch := time.J2000

	cases := []struct {
		name string
		a    float64
		e    float64
		ok   bool
	}{
		{"a main-belt orbit", 2.77, 0.076, true},
		{"a very eccentric but still closed orbit", 17.8, 0.967, true},

		// e >= 1 is not a tight orbit, it is a different conic. Elliptical
		// two-body propagation cannot represent it at all, so this must
		// fail at construction rather than at first use — which is the
		// difference between an error naming the elements and one arriving
		// from inside a propagation loop.
		{"parabolic", 2.0, 1.0, false},
		{"hyperbolic", 2.0, 1.5, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := NewElements(epoch, c.a, c.e,
				angle.Deg(10), angle.Deg(80), angle.Deg(73), angle.Deg(274))

			if c.ok && err != nil {
				t.Fatalf("NewElements(a=%v, e=%v) = %v, want success", c.a, c.e, err)
			}

			if !c.ok && err == nil {
				t.Fatalf("NewElements(a=%v, e=%v) succeeded; e >= 1 is not an ellipse", c.a, c.e)
			}
		})
	}
}

func TestNewFromElementsAnswersTheRegisteredBody(t *testing.T) {
	el, err := NewElements(time.J2000, 2.7658, 0.07839,
		angle.Deg(10.587), angle.Deg(80.393), angle.Deg(73.597), angle.Deg(77.372))
	if err != nil {
		t.Fatalf("NewElements: %v", err)
	}

	const ceres = core.ID(2000001)

	p, err := NewFromElements(ceres, el)
	if err != nil {
		t.Fatalf("NewFromElements: %v", err)
	}

	defer func() { _ = p.Close() }()

	st, err := p.State(ceres, time.J2000)
	if err != nil {
		t.Fatalf("State(registered body): %v", err)
	}

	// Ceres is a main-belt body: geocentric distance has to be of that
	// order. A bound this loose still catches a propagation that returns
	// the origin, the Sun, or metres instead of AU.
	if r := st.Pos.Norm(); r < 1.0 || r > 4.0 {
		t.Errorf("|r| = %v AU for a main-belt body at J2000", r)
	}

	// A body it does not propagate must still be answerable, through the
	// default SOFA base — that is what makes the returned Provider a drop-in
	// for a kernel-backed one.
	if _, err := p.State(core.Sun, time.J2000); err != nil {
		t.Errorf("State(Sun) through the default base: %v", err)
	}
}

// TestNewFromElementsRejectsBadElements covers the error path: an invalid
// registration must not yield a provider that fails later.
func TestNewFromElementsRejectsBadElements(t *testing.T) {
	if _, err := NewFromElements(core.ID(2000001), Elements{}); err == nil {
		t.Fatal("NewFromElements accepted a zero Elements")
	}
}

// TestWithKeplerBaseIsUsed proves the option reaches the propagation: a body
// the Kepler provider does not carry has to come from the supplied base.
func TestWithKeplerBaseIsUsed(t *testing.T) {
	base := &stubProvider{id: core.Sun, pos: [3]float64{1, 2, 3}}

	p := NewMovingBodyProvider(WithKeplerBase(base))

	st, err := p.State(core.Sun, time.J2000)
	if err != nil {
		t.Fatalf("State(Sun): %v", err)
	}

	if base.calls == 0 {
		t.Fatal("the supplied base was never called")
	}

	if st.Pos.X != 1 || st.Pos.Y != 2 || st.Pos.Z != 3 {
		t.Errorf("State(Sun).Pos = %v, want the base's answer {1 2 3}", st.Pos)
	}
}

// TestFreshMovingBodyProviderAnswersPluto covers the gap that used to exist
// here, and the reason it is gone.
//
// SOFA has no analytical Pluto, so kepler's default base could not place it
// and only Default() closed that by registering Pluto's elements. That left a
// satellite of Pluto — Charon — impossible to place from a plain provider,
// since a satellite is composed through its parent. The base now propagates
// the same Standish elements itself.
func TestFreshMovingBodyProviderAnswersPluto(t *testing.T) {
	p := NewMovingBodyProvider()

	st, err := p.State(core.Pluto, time.J2000)
	if err != nil {
		t.Fatalf("State(Pluto): %v", err)
	}

	// Pluto was near 31 AU from the Sun at J2000; geocentric distance is of
	// the same order. A bound this loose still catches the origin, the Sun,
	// or a body confused with another planet.
	if r := st.Pos.Norm(); r < 25 || r > 40 {
		t.Errorf("Pluto is %v AU away at J2000, expected roughly 31", r)
	}

	// Every SOFA-covered body still works, which is the other half.
	if _, err := p.State(core.Mars, time.J2000); err != nil {
		t.Errorf("State(Mars): %v", err)
	}
}

// TestDefaultAnswersEveryNamedBody checks Default's own doc claim: "No named
// core.ID this library defines returns ErrUnsupportedBody from here."
//
// Worth asserting rather than trusting, because it is exactly the kind of
// statement that stops being true when an identifier is added.
func TestDefaultAnswersEveryNamedBody(t *testing.T) {
	p := Default()

	defer func() { _ = p.Close() }()

	named := []core.ID{
		core.Mercury, core.Venus, core.Earth, core.Mars, core.Jupiter,
		core.Saturn, core.Uranus, core.Neptune, core.Pluto,
		core.Moon, core.Sun, core.SolarSystemBarycenter,
	}

	for _, id := range named {
		st, err := p.State(id, time.J2000)
		if err != nil {
			t.Errorf("Default().State(%s) = %v; the doc comment says every named body is answerable", id, err)

			continue
		}

		if math.IsNaN(st.Pos.X) || math.IsNaN(st.Pos.Y) || math.IsNaN(st.Pos.Z) {
			t.Errorf("Default().State(%s) returned NaN", id)
		}
	}
}

func TestNewProviderRejectsWhatItCannotBuild(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name   string
		source Source
		opts   []Option
		want   error
	}{
		{"satellites without a TLE", core.Satellites, nil, ErrTLERequired},
		{"stations", core.Stations, nil, ErrNotImplemented},
		{"a source that does not exist", Source("nonsense"), nil, ErrUnknownSource},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := NewProvider(ctx, c.source, "whatever", c.opts...)
			if !errors.Is(err, c.want) {
				t.Fatalf("NewProvider(%q) = %v, want %v", c.source, err, c.want)
			}
		})
	}
}

// TestNewProviderBuildsASatelliteOffline is the one NewProvider branch that
// needs no download, so it is the one that can assert the factory actually
// produces a working provider rather than only that it rejects bad input.
func TestNewProviderBuildsASatelliteOffline(t *testing.T) {
	// ISS (ZARYA), 2026-08-29 — the elements astrogo's own catalog/norad
	// live test logged, used here as a fixed fixture rather than fetched.
	const (
		line1 = "1 25544U 98067A   26241.27263584  .00006962  00000-0  13475-3 0  9998"
		line2 = "2 25544  51.6317 298.3548 0005039  86.9077 273.2487 15.48926826583086"
	)

	p, err := NewProvider(context.Background(), core.Satellites, "ISS", WithTLE(line1, line2))
	if err != nil {
		t.Fatalf("NewProvider(Satellites): %v", err)
	}

	defer func() { _ = p.Close() }()

	when := time.FromJD(2461281.5, time.UTC)

	// core.ID(0), not the NAIF catalogue number: a satellite provider tracks
	// exactly one object and refuses any other id, so that a Sun lookup
	// cannot be silently answered with the satellite's own state.
	alt, err := Altitude(p, core.ID(0), when)
	if err != nil {
		t.Fatalf("Altitude: %v", err)
	}

	// The ISS orbits between roughly 370 and 460 km. A bound that wide still
	// catches an altitude computed without subtracting the Earth's radius,
	// or one left in AU.
	if alt < 300 || alt > 600 {
		t.Errorf("ISS altitude = %.1f km, want roughly 400", alt)
	}
}

// TestWithTLERequiresBothLines: one line is not half a TLE, it is not a TLE.
func TestWithTLERequiresBothLines(t *testing.T) {
	_, err := NewProvider(context.Background(), core.Satellites, "ISS",
		WithTLE("1 25544U 98067A   26241.27263584  .00006962  00000-0  13475-3 0  9998", ""))
	if !errors.Is(err, ErrTLERequired) {
		t.Fatalf("NewProvider with one TLE line = %v, want ErrTLERequired", err)
	}
}

// TestOptionsRecordWhatTheySay covers the two options whose effect is only
// visible on the kernel-backed path, which needs a download.
//
// Checking the configuration they build is not a substitute for exercising
// that path — ephemeris/jpl's network suites do that — but it does catch the
// mistake these options are actually exposed to: an option that silently does
// nothing, or writes the wrong field.
func TestOptionsRecordWhatTheySay(t *testing.T) {
	start := time.FromJD(2460310.5, time.TDB)
	end := time.FromJD(2460350.5, time.TDB)

	var cfg config

	for _, opt := range []Option{
		WithTimeInterval(start, end),
		WithKernel("de441_part-2"),
		WithKernel("de441_part-3"),
	} {
		opt(&cfg)
	}

	if cfg.Start != start || cfg.End != end {
		t.Errorf("WithTimeInterval recorded (%v, %v), want (%v, %v)", cfg.Start, cfg.End, start, end)
	}

	// Chaining is documented on WithKernel, so appending rather than
	// overwriting is the behaviour under test.
	if len(cfg.ExtraKernels) != 2 ||
		cfg.ExtraKernels[0] != "de441_part-2" || cfg.ExtraKernels[1] != "de441_part-3" {
		t.Errorf("WithKernel recorded %v, want both kernels in order", cfg.ExtraKernels)
	}
}

func TestPositionAndVelocityAgreeWithState(t *testing.T) {
	p := Default()

	defer func() { _ = p.Close() }()

	st, err := p.State(core.Mars, time.J2000)
	if err != nil {
		t.Fatalf("State: %v", err)
	}

	pos, err := Position(p, core.Mars, time.J2000)
	if err != nil {
		t.Fatalf("Position: %v", err)
	}

	vel, err := Velocity(p, core.Mars, time.J2000)
	if err != nil {
		t.Fatalf("Velocity: %v", err)
	}

	if pos != st.Pos {
		t.Errorf("Position = %v, State.Pos = %v", pos, st.Pos)
	}

	if vel != st.Vel {
		t.Errorf("Velocity = %v, State.Vel = %v", vel, st.Vel)
	}
}

// TestHelpersPropagateTheProviderError keeps the three helpers from turning a
// failure into a zero value, which for a position is indistinguishable from
// the geocentre.
func TestHelpersPropagateTheProviderError(t *testing.T) {
	p := &stubProvider{id: core.Sun}

	const absent = core.ID(999999)

	if _, err := Position(p, absent, time.J2000); err == nil {
		t.Error("Position returned nil error for an unsupported body")
	}

	if _, err := Velocity(p, absent, time.J2000); err == nil {
		t.Error("Velocity returned nil error for an unsupported body")
	}

	if _, err := Altitude(p, absent, time.J2000); err == nil {
		t.Error("Altitude returned nil error for an unsupported body")
	}
}

func TestToICRS(t *testing.T) {
	cases := []struct {
		name    string
		pos     vector.Vec3
		wantRA  float64
		wantDec float64
	}{
		{"along +X is the origin of right ascension", vector.Vec3{X: 1}, 0, 0},
		{"along +Y is six hours", vector.Vec3{Y: 1}, 90, 0},
		{"along +Z is the north celestial pole", vector.Vec3{Z: 1}, 0, 90},

		// The wrap is the part worth pinning: -Y must come back as 270°,
		// not -90°, or every consumer formatting an RA gets a negative hour
		// angle.
		{"along -Y wraps rather than going negative", vector.Vec3{Y: -1}, 270, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ToICRS(c.pos)
			if err != nil {
				t.Fatalf("ToICRS: %v", err)
			}

			if d := math.Abs(got.RA().Degrees() - c.wantRA); d > 1e-9 && c.wantDec != 90 {
				t.Errorf("RA = %v deg, want %v", got.RA().Degrees(), c.wantRA)
			}

			if d := math.Abs(got.Dec().Degrees() - c.wantDec); d > 1e-9 {
				t.Errorf("Dec = %v deg, want %v", got.Dec().Degrees(), c.wantDec)
			}
		})
	}
}

// TestToICRSRejectsTheZeroVector: a direction cannot be recovered from no
// direction, and returning the pole would be a plausible-looking lie.
func TestToICRSRejectsTheZeroVector(t *testing.T) {
	if _, err := ToICRS(vector.Vec3{}); !errors.Is(err, ErrZeroVector) {
		t.Fatalf("ToICRS(zero) = %v, want ErrZeroVector", err)
	}
}

// TestDefaultCloseIsSafe covers the other half of a Provider's contract: a
// caller that defers Close must not be punished for it.
func TestDefaultCloseIsSafe(t *testing.T) {
	p := Default()

	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Closing an offline provider twice is not an error either; nothing
	// holds an OS resource.
	if err := p.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// TestSofaProviderCloseIsANoOp: the internal SOFA source holds no kernel,
// no file and no connection, so closing it must succeed rather than being
// left unimplemented — Default wraps it, and a caller closing the wrapper
// should not have to know which layer owns a resource.
func TestSofaProviderCloseIsANoOp(t *testing.T) {
	s := &sofaProvider{}
	if err := s.Close(); err != nil {
		t.Fatalf("sofaProvider.Close: %v", err)
	}
}
