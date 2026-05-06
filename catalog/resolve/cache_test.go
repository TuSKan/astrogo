package resolve_test

import (
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

func TestMapCacheWriteRead(t *testing.T) {
	cache := resolve.NewMapCache()
	defer cache.Close()

	key := "test:query:1"

	_, ok := cache.Get(key)
	testutil.AssertEqual(t, "Initial Miss", ok, false)

	targets := []resolve.Target{
		{
			ID:          "OBJ1",
			Name:        "Test Object",
			Designation: "Desig1",
			SPKID:       "999",
			Kind:        resolve.KindStar,
			Catalog:     "test",
			Coord:       coord.NewICRS(angle.Deg(45.0), angle.Deg(-15.0)),
			HasCoord:    true,
			Aliases:     []string{"A", "B"},
		},
		{
			ID:      "OBJ2", // Minimal resolve.Target checking nil coordinates
			Catalog: "test",
		},
	}

	err := cache.Set(key, targets)
	testutil.AssertNoError(t, err)

	seq, ok := cache.Get(key)
	testutil.AssertEqual(t, "Cache Hit", ok, true)

	var retrieved []resolve.Target
	seq(func(tar resolve.Target, err error) bool {
		testutil.AssertNoError(t, err)
		retrieved = append(retrieved, tar)
		return true
	})

	if len(retrieved) != 2 {
		t.Fatalf("Expected 2 resolve.Targets, got %d", len(retrieved))
	}

	// Validate full decode matches full encode
	t1 := retrieved[0]
	testutil.AssertEqual(t, "ID", t1.ID, "OBJ1")
	testutil.AssertEqual(t, "Name", t1.Name, "Test Object")
	testutil.AssertEqual(t, "Designation", t1.Designation, "Desig1")
	testutil.AssertEqual(t, "SPKID", t1.SPKID, "999")
	testutil.AssertEqual(t, "Kind", string(t1.Kind), string(resolve.KindStar))
	if !t1.HasCoord {
		t.Fatalf("Expected coordinate, got no coord")
	}
	if math.Abs(t1.Coord.RA().Degrees()-45.0) > 1e-9 {
		t.Errorf("RA mismatch: got %f", t1.Coord.RA().Degrees())
	}
	if math.Abs(t1.Coord.Dec().Degrees() - -15.0) > 1e-9 {
		t.Errorf("Dec mismatch: got %f", t1.Coord.Dec().Degrees())
	}
	testutil.AssertEqual(t, "Aliases", strings.Join(t1.Aliases, ","), "A,B")

	// Validate partial decode
	t2 := retrieved[1]
	testutil.AssertEqual(t, "ID 2", t2.ID, "OBJ2")
	if t2.HasCoord {
		t.Fatalf("Expected no coordinate for OBJ2, got %v", t2.Coord)
	}

	// Overwrite validation
	err = cache.Set(key, []resolve.Target{{ID: "OVERWRITTEN"}})
	testutil.AssertNoError(t, err)

	seq2, _ := cache.Get(key)
	var o []resolve.Target
	seq2(func(tar resolve.Target, err error) bool {
		o = append(o, tar)
		return true
	})
	testutil.AssertEqual(t, "Overwrite Length", len(o), 1)
	testutil.AssertEqual(t, "Overwritten ID", o[0].ID, "OVERWRITTEN")
}

func TestMapCacheRelease(t *testing.T) {
	cache := resolve.NewMapCache()
	err := cache.Set("key1", []resolve.Target{{ID: "A"}})
	testutil.AssertNoError(t, err)

	// Ensure Close clears maps safely unlocking Arrow memory boundaries
	err = cache.Close()
	testutil.AssertNoError(t, err)

	_, ok := cache.Get("key1")
	testutil.AssertEqual(t, "Cache Hit After Close", ok, false)
}
