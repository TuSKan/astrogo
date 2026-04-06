package catalog

import (
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
)

func TestArrowCacheWriteRead(t *testing.T) {
	cache := NewArrowCache()
	defer cache.Close()

	key := "test:query:1"

	_, ok := cache.Get(key)
	testutil.AssertEqual(t, "Initial Miss", ok, false)

	targets := []Target{
		{
			ID:          "OBJ1",
			Name:        "Test Object",
			Designation: "Desig1",
			SPKID:       "999",
			Kind:        KindStar,
			Catalog:     "test",
			Coord:       coord.NewICRS(angle.Deg(45.0), angle.Deg(-15.0)),
			Aliases:     []string{"A", "B"},
		},
		{
			ID:      "OBJ2", // Minimal target checking nil coordinates
			Catalog: "test",
		},
	}

	err := cache.Set(key, targets)
	testutil.AssertNoError(t, err)

	seq, ok := cache.Get(key)
	testutil.AssertEqual(t, "Cache Hit", ok, true)

	var retrieved []Target
	seq(func(tar Target, err error) bool {
		testutil.AssertNoError(t, err)
		retrieved = append(retrieved, tar)
		return true
	})

	if len(retrieved) != 2 {
		t.Fatalf("Expected 2 targets, got %d", len(retrieved))
	}

	// Validate full decode matches full encode
	t1 := retrieved[0]
	testutil.AssertEqual(t, "ID", t1.ID, "OBJ1")
	testutil.AssertEqual(t, "Name", t1.Name, "Test Object")
	testutil.AssertEqual(t, "Designation", t1.Designation, "Desig1")
	testutil.AssertEqual(t, "SPKID", t1.SPKID, "999")
	testutil.AssertEqual(t, "Kind", string(t1.Kind), string(KindStar))
	if t1.Coord == nil {
		t.Fatalf("Expected coordinate, got nil")
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
	if t2.Coord != nil {
		t.Fatalf("Expected nil coordinate for OBJ2, got %v", t2.Coord)
	}

	// Overwrite validation
	err = cache.Set(key, []Target{{ID: "OVERWRITTEN"}})
	testutil.AssertNoError(t, err)

	seq2, _ := cache.Get(key)
	var o []Target
	seq2(func(tar Target, err error) bool {
		o = append(o, tar)
		return true
	})
	testutil.AssertEqual(t, "Overwrite Length", len(o), 1)
	testutil.AssertEqual(t, "Overwritten ID", o[0].ID, "OVERWRITTEN")
}

func TestArrowCacheRelease(t *testing.T) {
	cache := NewArrowCache()
	err := cache.Set("key1", []Target{{ID: "A"}})
	testutil.AssertNoError(t, err)

	// Ensure Close clears maps safely unlocking Arrow memory boundaries
	err = cache.Close()
	testutil.AssertNoError(t, err)

	_, ok := cache.Get("key1")
	testutil.AssertEqual(t, "Cache Hit After Close", ok, false)
}
