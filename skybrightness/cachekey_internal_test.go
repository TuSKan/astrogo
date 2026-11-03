package skybrightness

import (
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
)

// uncomparableProvider is a provider whose dynamic type == would panic on,
// because of the slice it carries.
type uncomparableProvider struct {
	eph.Provider

	tags []string
}

// TestSameProvider: identity, and never a panic.
func TestSameProvider(t *testing.T) {
	t.Parallel()

	a, b := eph.Default(), eph.Default()
	odd := uncomparableProvider{Provider: a, tags: []string{"x"}}

	for _, c := range []struct {
		name string
		x, y eph.Provider
		want bool
	}{
		{"the same provider", a, a, true},
		{"two providers built alike", a, b, false},
		{"neither set", nil, nil, true},
		{"one set", a, nil, false},
		{"an uncomparable type, against itself", odd, odd, false},
		{"an uncomparable type, against another", odd, a, false},
	} {
		if got := sameProvider(c.x, c.y); got != c.want {
			t.Errorf("%s: sameProvider = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestMoonGeometryIsKeyedOnTheEphemeris: every position in the cached
// geometry came from the scene's ephemeris, so another one must rebuild it,
// and the same one must not (#945).
func TestMoonGeometryIsKeyedOnTheEphemeris(t *testing.T) {
	t.Parallel()

	m := flatMoonlight(t)
	scene := paranalMoonScene(t, 7, 46)

	first, err := m.geometry(scene)
	if err != nil {
		t.Fatalf("geometry: %v", err)
	}

	if again, err := m.geometry(scene); err != nil || again != first {
		t.Fatalf("the same scene rebuilt the geometry (err %v)", err)
	}

	other := *scene
	other.Ephemeris = eph.Default()

	if rebuilt, err := m.geometry(&other); err != nil || rebuilt == first {
		t.Errorf("another ephemeris reused the cached geometry (err %v)", err)
	}
}

// TestSceneFrameIsKeyedOnTheEphemerisWhenItHoldsTheSun: the frame's Sun came
// from the scene's ephemeris. A frame without one does not depend on it and
// keeps matching (#945).
func TestSceneFrameIsKeyedOnTheEphemerisWhenItHoldsTheSun(t *testing.T) {
	t.Parallel()

	scene := paranalMoonScene(t, 7, 46)

	other := *scene
	other.Ephemeris = eph.Default()

	for _, needSun := range []bool{true, false} {
		cache := &frameCache{needSun: needSun}

		first, err := cache.get(scene)
		if err != nil {
			t.Fatalf("needSun %v: get: %v", needSun, err)
		}

		if again, err := cache.get(scene); err != nil || again != first {
			t.Fatalf("needSun %v: the same scene rebuilt the frame (err %v)", needSun, err)
		}

		second, err := cache.get(&other)
		if err != nil {
			t.Fatalf("needSun %v: get: %v", needSun, err)
		}

		if rebuilt := second != first; rebuilt != needSun {
			t.Errorf("needSun %v: another ephemeris rebuilt the frame = %v, want %v",
				needSun, rebuilt, needSun)
		}
	}
}
