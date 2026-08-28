package fits

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
)

// A position read from a column that had none is not a position.
//
// GetFloatColumn writes the zero value for a null entry, so a row with no
// coordinates arrives as 0.0 rather than as an error. Setting HasCoord on that
// row puts a target on the equator at the equinox — a real place in Cetus,
// which a scheduler will happily slew to.
//
// The out-of-range cases are the substitutions a hand-built table makes:
// right ascension written in hours or in radians rather than degrees, and a
// declination that is not an angle at all.
func TestUsableCoord(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name    string
		ra, dec float64
		want    bool
	}{
		// Real positions.
		{"an ordinary target", 266.405, -28.936, true},
		{"the north celestial pole", 0, 90, true},
		{"the south celestial pole", 0, -90, true},
		{"right ascension at the wrap", 359.9999, 12, true},
		{"right ascension in the signed convention", -170, -45, true},
		{"on the equator but not at the equinox", 180, 0, true},
		{"at the equinox but not on the equator", 0, 0.0001, true},

		// Not positions.
		{"both null", 0, 0, false},
		{"right ascension not a number", math.NaN(), 45, false},
		{"declination not a number", 120, math.NaN(), false},
		{"right ascension infinite", math.Inf(1), 45, false},
		{"declination infinite", 120, math.Inf(-1), false},
		{"declination past the pole", 120, 91, false},
		{"declination past the other pole", 120, -90.5, false},
		{"declination as a right ascension", 120, 275, false},
		{"right ascension in hours", 400, 45, false},
		{"right ascension far out of range", -1000, 45, false},
	} {
		if got := usableCoord(c.ra, c.dec); got != c.want {
			t.Errorf("%s: usableCoord(%v, %v) = %v, want %v", c.name, c.ra, c.dec, got, c.want)
		}
	}
}

// A row without a usable position keeps everything else it had. Dropping the
// row would lose a name that resolves; keeping it with a fabricated position
// is worse than either.
func TestTargetWithoutCoordKeepsItsIdentity(t *testing.T) {
	t.Parallel()

	p := &Provider{
		name: "synthetic",
		targets: []resolve.Target{
			{ID: "OBJ-1", Name: "Named But Unplaced", Kind: resolve.KindOther, Catalog: "FITS"},
			{
				ID: "OBJ-2", Name: "Named And Placed", Kind: resolve.KindOther, Catalog: "FITS",
				Coord:    coord.NewICRS(angle.Deg(83.822), angle.Deg(-5.391)),
				HasCoord: true,
			},
		},
	}

	got, ok := p.Resolve("Named But Unplaced")
	if !ok {
		t.Fatal("a target with no position stopped resolving by name")
	}

	if got.HasCoord {
		t.Error("a target built without a position reports HasCoord")
	}

	if got.ID != "OBJ-1" {
		t.Errorf("ID = %q, want OBJ-1", got.ID)
	}

	// And one that does have a position still carries it.
	placed, ok := p.Resolve("Named And Placed")
	if !ok {
		t.Fatal("a target with a position stopped resolving")
	}

	if !placed.HasCoord {
		t.Error("a target built with a position does not report HasCoord")
	}

	// Both are still searchable, so the guard costs no discoverability.
	if n := len(p.Search("named")); n != 2 {
		t.Errorf("a substring search matched %d targets, want 2", n)
	}
}
