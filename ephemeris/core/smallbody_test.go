package core

import (
	"fmt"
	"testing"
)

// TestSmallBodyIDsDoNotCollideWithNamedBodies is the property this exists
// for. Asteroid numbers 1 through 12 used to land exactly on the named body
// identifiers — 4 Vesta on Mars, 1 Ceres on Mercury — so a provider holding
// a planetary kernel dropped the asteroid as a duplicate.
func TestSmallBodyIDsDoNotCollideWithNamedBodies(t *testing.T) {
	named := make([]ID, 0, len(idNames))
	for id := range idNames {
		named = append(named, id)
	}

	for n := 1; n <= 20; n++ {
		got, _ := SmallBodyID(n)

		for _, id := range named {
			if got == id {
				t.Errorf("SmallBodyID(%d) = %d, which collides with %s", n, got, id)
			}
		}
	}
}

func TestSmallBodyNumberRoundTrip(t *testing.T) {
	for _, n := range []int{1, 4, 12, 16, 433, 624, 3200, 101955} {
		id, _ := SmallBodyID(n)

		got, ok := id.SmallBodyNumber()
		if !ok {
			t.Errorf("SmallBodyID(%d).SmallBodyNumber() reported not-a-small-body", n)

			continue
		}

		if got != n {
			t.Errorf("SmallBodyID(%d) round-tripped to %d", n, got)
		}
	}
}

// TestSmallBodyNumberRejectsOtherIDs keeps the accessor from claiming
// identifiers that are not small bodies — a named body, and the comet range,
// which NAIF puts elsewhere.
func TestSmallBodyNumberRejectsOtherIDs(t *testing.T) {
	cases := []ID{Mercury, Mars, Sun, SolarSystemBarycenter, 0, 433, 1000036, SmallBodyBase}

	for _, id := range cases {
		if n, ok := id.SmallBodyNumber(); ok {
			t.Errorf("ID(%d).SmallBodyNumber() = %d, true; want not-a-small-body", int32(id), n)
		}
	}
}

func TestSmallBodyIDString(t *testing.T) {
	eros, _ := SmallBodyID(433)
	vesta, _ := SmallBodyID(4)

	if got := eros.String(); got != "SmallBody(433)" {
		t.Errorf("SmallBodyID(433).String() = %q, want %q", got, "SmallBody(433)")
	}

	// Vesta must not print as Mars, which is what the collision did.
	if got := vesta.String(); got == Mars.String() {
		t.Errorf("SmallBodyID(4) prints as %q, the same as Mars", got)
	}

	// A named body is unaffected.
	if got := Mars.String(); got != "Mars" {
		t.Errorf("Mars.String() = %q", got)
	}
}

func ExampleSmallBodyID() {
	eros, _ := SmallBodyID(433)
	vesta, _ := SmallBodyID(4)

	fmt.Println(eros)
	fmt.Println(vesta, "not", Mars)
	// Output:
	// SmallBody(433)
	// SmallBody(4) not Mars
}

// TestSmallBodyIDRejectsOutOfRange keeps an impossible number from aliasing
// a real body: returning SmallBodyBase+n for a negative n would wrap into
// the identifier space of something else entirely, and returning 0 would be
// the Solar System barycenter (#809).
func TestSmallBodyIDRejectsOutOfRange(t *testing.T) {
	for _, n := range []int{0, -1, -433, 1000000, 1 << 40} {
		if got, ok := SmallBodyID(n); ok {
			t.Errorf("SmallBodyID(%d) = %d, true; want not a small-body number", n, got)
		}
	}
}
