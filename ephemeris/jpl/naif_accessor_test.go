package jpl_test

import (
	"slices"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
)

// BodyIDToNAIF is an exported map, so any package could reassign an entry and
// change which SPK segment every other caller in the binary resolves to —
// silently, with nothing for a later reader to see. The accessors exist so
// reading it does not require reaching into that state.
func TestNAIFAccessors(t *testing.T) {
	t.Parallel()

	bodies := jpl.NAIFBodies()
	if len(bodies) == 0 {
		t.Fatal("no bodies mapped")
	}

	if !slices.IsSorted(bodies) {
		t.Errorf("bodies are not sorted, so the order depends on map iteration: %v", bodies)
	}

	// Everything enumerated must resolve, which is what makes the pair
	// usable together.
	for _, id := range bodies {
		if _, ok := jpl.NAIFFor(id); !ok {
			t.Errorf("NAIFBodies reported %v, which NAIFFor does not map", id)
		}
	}

	// A couple of anchors, so a wholesale renumbering would not pass.
	for id, want := range map[core.ID]int{core.Earth: 399, core.Moon: 301, core.Sun: 10} {
		got, ok := jpl.NAIFFor(id)
		if !ok || got != want {
			t.Errorf("NAIFFor(%v) = %d, %v; want %d, true", id, got, ok, want)
		}
	}

	if _, ok := jpl.NAIFFor(core.ID(999999)); ok {
		t.Error("NAIFFor reports an unmapped body as mapped")
	}
}
