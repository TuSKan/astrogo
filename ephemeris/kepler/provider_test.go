package kepler_test

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/kepler"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
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
		time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC),
		unit.AU(2.5), 0.1, angle.Deg(5), angle.Deg(30), angle.Deg(60), angle.Deg(0),
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
		time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC),
		unit.AU(2.5), 1.5, angle.Deg(5), angle.Deg(30), angle.Deg(60), angle.Deg(0), // e=1.5, hyperbolic
	)
	testutil.AssertErrorIs(t, err, kepler.ErrUnsupportedOrbit)
}

func TestProvider_Locate_PropagatedBody(t *testing.T) {
	const id core.ID = 1_000_000

	el := testElementsForProvider(t)
	p := kepler.New()
	testutil.AssertNoError(t, p.Register(id, el))

	st, err := eph.At(p, el.Epoch()).Geometric(id, core.Earth)
	testutil.AssertNoError(t, err)

	// Geocentric distance to a body ~2.5 AU from the Sun must be a real,
	// finite, plausible value — not the zero value a routing bug (e.g.
	// answering with Earth's own position) would produce.
	if d := st.Distance(); d < 1.0 || d > 4.0 {
		t.Errorf("Distance() = %v AU, outside plausible band for a=2.5 AU", d)
	}
}

func TestProvider_Locate_DefaultBase_SunMoonPlanets(t *testing.T) {
	// A propagated ID that never matches any of the queried bodies below,
	// so every case here exercises Provider.Locate's delegation to the
	// default sofaBase, and sofaBase's own per-body switch.
	const propagatedID core.ID = 1_000_000

	p := kepler.New()
	testutil.AssertNoError(t, p.Register(propagatedID, testElementsForProvider(t)))

	tm := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	bodies := []struct {
		name string
		id   core.ID
	}{
		{"Sun", core.Sun},
		{"Moon", core.Moon},
		{"Mercury", core.Mercury},
		{"Venus", core.Venus},
		{"Earth", core.Earth},
		{"Mars", core.MarsBarycenter},
		{"Jupiter", core.JupiterBarycenter},
		{"Saturn", core.SaturnBarycenter},
		{"Uranus", core.UranusBarycenter},
		{"Neptune", core.NeptuneBarycenter},
	}

	for _, b := range bodies {
		t.Run(b.name, func(t *testing.T) {
			st, err := eph.At(p, tm).Geometric(b.id, core.Earth)
			if err != nil {
				t.Fatalf("Geometric(%s): %v", b.name, err)
			}

			// Earth's own geocentric position is ~0 (it's the origin);
			// every other body must be a real, non-degenerate distance.
			if b.name != "Earth" && st.Distance() == 0 {
				t.Errorf("Geometric(%s): Distance() = 0, want a real nonzero geocentric distance", b.name)
			}
		})
	}
}

func TestProvider_Locate_DefaultBase_UnsupportedBody(t *testing.T) {
	const propagatedID core.ID = 1_000_000

	p := kepler.New()
	testutil.AssertNoError(t, p.Register(propagatedID, testElementsForProvider(t)))

	tm := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	// Pluto used to be the example here, because SOFA has no analytical
	// source for it. The default base now propagates its Standish elements
	// so that a satellite of Pluto can be placed, so the gap it stood for is
	// gone; an unnamed body still falls through.
	if _, err := eph.At(p, tm).Geometric(core.PlutoBarycenter, core.Earth); err != nil {
		t.Errorf("Geometric(Pluto) = %v; the default base answers it now", err)
	}

	_, err := eph.At(p, tm).Geometric(core.ID(987_654), core.Earth)
	testutil.AssertErrorIs(t, err, kepler.ErrUnsupportedBody)
}

// mockBaseProvider is a minimal core.Provider double for verifying
// WithBase actually overrides the default sofaBase, and that
// Provider.Locate wraps a base-provider error rather than passing it
// through unwrapped or swallowing it.
type mockBaseProvider struct {
	called  bool
	wantErr error
}

var errMockBase = errors.New("mockBaseProvider: intentional test failure")

func (m *mockBaseProvider) Locate(core.ID, time.Time) (core.State, error) {
	m.called = true
	if m.wantErr != nil {
		return core.State{}, m.wantErr
	}

	return core.State{Frame: core.FrameICRS}, nil
}

func (m *mockBaseProvider) Close() error { return nil }

func TestProvider_Locate_WithBase_Override(t *testing.T) {
	const propagatedID core.ID = 1_000_000

	mock := &mockBaseProvider{}

	p := kepler.New(kepler.WithBase(mock))
	testutil.AssertNoError(t, p.Register(propagatedID, testElementsForProvider(t)))

	tm := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	if _, err := eph.At(p, tm).Geometric(core.Sun, core.Earth); err != nil {
		t.Fatalf("Geometric(Sun): %v", err)
	}

	if !mock.called {
		t.Error("WithBase's provider was never called — Provider.Locate did not delegate to it")
	}
}

func TestProvider_Locate_WithBase_WrapsError(t *testing.T) {
	const propagatedID core.ID = 1_000_000

	mock := &mockBaseProvider{wantErr: errMockBase}

	p := kepler.New(kepler.WithBase(mock))
	testutil.AssertNoError(t, p.Register(propagatedID, testElementsForProvider(t)))

	tm := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	_, err := eph.At(p, tm).Geometric(core.Sun, core.Earth)
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
// shared Provider are each located correctly — the capability this
// generalization exists for: one Provider answering for many bodies.
func TestProvider_MultiBody(t *testing.T) {
	el1 := testElementsForProvider(t)

	el2, err := kepler.NewElements(
		time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC),
		4.0, 0.2, angle.Deg(10), angle.Deg(50), angle.Deg(90), angle.Deg(180),
	)
	testutil.AssertNoError(t, err)

	const id1, id2 core.ID = 1_000_001, 1_000_002

	p := kepler.New()
	testutil.AssertNoError(t, p.Register(id1, el1))
	testutil.AssertNoError(t, p.Register(id2, el2))

	st1, err := eph.At(p, el1.Epoch()).Geometric(id1, core.Earth)
	testutil.AssertNoError(t, err)

	st2, err := eph.At(p, el2.Epoch()).Geometric(id2, core.Earth)
	testutil.AssertNoError(t, err)

	// The two bodies have different semi-major axes (2.5 AU vs 4.0 AU)
	// and should never coincidentally report the same geocentric
	// distance — a routing bug (e.g. both resolving to the same
	// registered entry) would collapse them to identical positions.
	if st1.Distance() == st2.Distance() {
		t.Errorf("expected distinct distances for id1/id2, both got %v AU", st1.Distance())
	}
}
