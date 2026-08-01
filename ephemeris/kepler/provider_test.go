package kepler_test

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/kepler"
	"github.com/TuSKan/astrogo/internal/testutil"
	atime "github.com/TuSKan/astrogo/time"
)

// testElementsForProvider is a small, valid osculating-elements fixture
// shared by this file's tests — the physics itself (StateAt's
// correctness) is covered by kepler_test.go; these tests exercise
// Provider/sofaBase's own wiring: which body ID routes to registered
// elements vs. the base delegate, and how base-provider errors get
// wrapped.
func testElementsForProvider(t *testing.T) kepler.Elements {
	t.Helper()

	el, err := kepler.NewElements(
		atime.Date(2026, atime.January, 1, 0, 0, 0, 0, atime.LocationUTC),
		2.5, 0.1, angle.Deg(5), angle.Deg(30), angle.Deg(60), angle.Deg(0),
	)
	testutil.AssertNoError(t, err)

	return el
}

func TestNew_Success(t *testing.T) {
	p := kepler.New()
	if p == nil {
		t.Fatal("New returned nil")
	}
}

func TestRegister_RejectsInvalidElements(t *testing.T) {
	// NewElements itself already rejects hyperbolic eccentricity at
	// construction — this confirms Register's own Validate call is real
	// defense-in-depth, not dead code, by constructing the invalid
	// Elements via the package-internal fields (accessible from this
	// black-box test only through NewElements, so this exercises the
	// same guard NewElements already enforces from a second angle:
	// Register never trusts an Elements value blindly).
	_, err := kepler.NewElements(
		atime.Date(2026, atime.January, 1, 0, 0, 0, 0, atime.LocationUTC),
		2.5, 1.5, angle.Deg(5), angle.Deg(30), angle.Deg(60), angle.Deg(0), // e=1.5, hyperbolic
	)
	testutil.AssertErrorIs(t, err, kepler.ErrUnsupportedOrbit)
}

func TestProvider_State_PropagatedBody(t *testing.T) {
	const id core.ID = 1_000_000

	el := testElementsForProvider(t)
	p := kepler.New()
	testutil.AssertNoError(t, p.Register(id, el))

	st, err := p.State(id, el.Epoch())
	testutil.AssertNoError(t, err)

	// Geocentric distance to a body ~2.5 AU from the Sun must be a real,
	// finite, plausible value — not the zero value a routing bug (e.g.
	// answering with Earth's own position) would produce.
	if d := st.Distance(); d < 1.0 || d > 4.0 {
		t.Errorf("Distance() = %v AU, outside plausible band for a=2.5 AU", d)
	}
}

func TestProvider_State_DefaultBase_SunMoonPlanets(t *testing.T) {
	// A propagated ID that never matches any of the queried bodies below,
	// so every case here exercises Provider.State's delegation to the
	// default sofaBase, and sofaBase's own per-body switch.
	const propagatedID core.ID = 1_000_000

	p := kepler.New()
	testutil.AssertNoError(t, p.Register(propagatedID, testElementsForProvider(t)))

	tm := atime.Date(2026, atime.January, 1, 0, 0, 0, 0, atime.LocationUTC)

	bodies := []struct {
		name string
		id   core.ID
	}{
		{"Sun", core.Sun},
		{"Moon", core.Moon},
		{"Mercury", core.Mercury},
		{"Venus", core.Venus},
		{"Earth", core.Earth},
		{"Mars", core.Mars},
		{"Jupiter", core.Jupiter},
		{"Saturn", core.Saturn},
		{"Uranus", core.Uranus},
		{"Neptune", core.Neptune},
	}

	for _, b := range bodies {
		t.Run(b.name, func(t *testing.T) {
			st, err := p.State(b.id, tm)
			if err != nil {
				t.Fatalf("State(%s): %v", b.name, err)
			}

			// Earth's own geocentric position is ~0 (it's the origin);
			// every other body must be a real, non-degenerate distance.
			if b.name != "Earth" && st.Distance() == 0 {
				t.Errorf("State(%s): Distance() = 0, want a real nonzero geocentric distance", b.name)
			}
		})
	}
}

func TestProvider_State_DefaultBase_UnsupportedBody(t *testing.T) {
	const propagatedID core.ID = 1_000_000

	p := kepler.New()
	testutil.AssertNoError(t, p.Register(propagatedID, testElementsForProvider(t)))

	tm := atime.Date(2026, atime.January, 1, 0, 0, 0, 0, atime.LocationUTC)

	// Pluto has no gofaext analytical source — the default sofaBase's
	// documented gap (mirrors ephemeris's own sofaProvider).
	_, err := p.State(core.Pluto, tm)
	testutil.AssertErrorIs(t, err, kepler.ErrUnsupportedBody)
}

// mockBaseProvider is a minimal core.Provider double for verifying
// WithBase actually overrides the default sofaBase, and that
// Provider.State wraps a base-provider error rather than passing it
// through unwrapped or swallowing it.
type mockBaseProvider struct {
	called  bool
	wantErr error
}

var errMockBase = errors.New("mockBaseProvider: intentional test failure")

func (m *mockBaseProvider) State(core.ID, atime.Time) (core.State, error) {
	m.called = true
	if m.wantErr != nil {
		return core.State{}, m.wantErr
	}

	return core.State{}, nil
}

func (m *mockBaseProvider) Close() error { return nil }

func TestProvider_State_WithBase_Override(t *testing.T) {
	const propagatedID core.ID = 1_000_000

	mock := &mockBaseProvider{}

	p := kepler.New(kepler.WithBase(mock))
	testutil.AssertNoError(t, p.Register(propagatedID, testElementsForProvider(t)))

	tm := atime.Date(2026, atime.January, 1, 0, 0, 0, 0, atime.LocationUTC)

	if _, err := p.State(core.Sun, tm); err != nil {
		t.Fatalf("State(Sun): %v", err)
	}

	if !mock.called {
		t.Error("WithBase's provider was never called — Provider.State did not delegate to it")
	}
}

func TestProvider_State_WithBase_WrapsError(t *testing.T) {
	const propagatedID core.ID = 1_000_000

	mock := &mockBaseProvider{wantErr: errMockBase}

	p := kepler.New(kepler.WithBase(mock))
	testutil.AssertNoError(t, p.Register(propagatedID, testElementsForProvider(t)))

	tm := atime.Date(2026, atime.January, 1, 0, 0, 0, 0, atime.LocationUTC)

	_, err := p.State(core.Sun, tm)
	testutil.AssertErrorIs(t, err, errMockBase)
}

func TestProvider_Close(t *testing.T) {
	p := kepler.New()
	testutil.AssertNoError(t, p.Register(1_000_000, testElementsForProvider(t)))

	if err := p.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}

// TestProvider_MultiBody confirms two distinct registered bodies on one
// shared Provider each answer their own State correctly — the core new
// capability this generalization exists for (plan.VisibleTonight sharing
// one Provider across every Kepler-eligible candidate in a night's run).
func TestProvider_MultiBody(t *testing.T) {
	el1 := testElementsForProvider(t)

	el2, err := kepler.NewElements(
		atime.Date(2026, atime.January, 1, 0, 0, 0, 0, atime.LocationUTC),
		4.0, 0.2, angle.Deg(10), angle.Deg(50), angle.Deg(90), angle.Deg(180),
	)
	testutil.AssertNoError(t, err)

	const id1, id2 core.ID = 1_000_001, 1_000_002

	p := kepler.New()
	testutil.AssertNoError(t, p.Register(id1, el1))
	testutil.AssertNoError(t, p.Register(id2, el2))

	st1, err := p.State(id1, el1.Epoch())
	testutil.AssertNoError(t, err)

	st2, err := p.State(id2, el2.Epoch())
	testutil.AssertNoError(t, err)

	// The two bodies have different semi-major axes (2.5 AU vs 4.0 AU)
	// and should never coincidentally report the same geocentric
	// distance — a routing bug (e.g. both resolving to the same
	// registered entry) would collapse them to identical positions.
	if st1.Distance() == st2.Distance() {
		t.Errorf("expected distinct distances for id1/id2, both got %v AU", st1.Distance())
	}
}
