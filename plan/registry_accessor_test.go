package plan_test

import (
	"slices"
	"testing"

	"github.com/TuSKan/astrogo/plan"
)

// The registries are exported maps, which makes them process-wide mutable
// state: one package deleting or replacing an entry changes what every other
// caller in the binary sees. That is a reproducibility and test-isolation
// problem rather than a race one, and it is why enumeration now goes through
// a function.
//
// The maps stay, deprecated, because this repository's own rule gives a
// deprecated symbol two minor releases before removal.
func TestKnownSiteNamesEnumeratesEveryEntry(t *testing.T) {
	t.Parallel()

	names := plan.KnownSiteNames()
	if len(names) == 0 {
		t.Fatal("no known sites")
	}

	if !slices.IsSorted(names) {
		t.Errorf("names are not sorted, so the order depends on map iteration: %v", names)
	}

	// Every name it reports must resolve, which is the property that makes
	// the pair useful: enumerate, then look up.
	for _, n := range names {
		if _, err := plan.NewKnownSite(n); err != nil {
			t.Errorf("KnownSiteNames reported %q, which NewKnownSite cannot resolve: %v", n, err)
		}
	}
}

func TestMeteorShowerNamesEnumeratesEveryEntry(t *testing.T) {
	t.Parallel()

	names := plan.MeteorShowerNames()
	if len(names) == 0 {
		t.Fatal("no meteor showers")
	}

	if !slices.IsSorted(names) {
		t.Errorf("names are not sorted: %v", names)
	}

	for _, n := range names {
		if _, err := plan.NewMeteorShower(n); err != nil {
			t.Errorf("MeteorShowerNames reported %q, which NewMeteorShower cannot resolve: %v", n, err)
		}
	}
}

// TwilightThreshold reports whether the kind is one this package defines,
// which the map lookup it replaces could only do by comma-ok at every call
// site.
func TestTwilightThresholdKnowsWhatItDefines(t *testing.T) {
	t.Parallel()

	for _, k := range []plan.TwilightKind{plan.CivilTwilight, plan.NauticalTwilight, plan.AstronomicalTwilight} {
		v, ok := plan.TwilightThreshold(k)
		if !ok {
			t.Errorf("TwilightThreshold(%v) reports the kind as undefined", k)
		}

		// Every twilight threshold is a negative solar altitude: the Sun is
		// below the horizon by definition.
		if v >= 0 {
			t.Errorf("TwilightThreshold(%v) = %v, want a negative solar altitude", k, v)
		}
	}

	if _, ok := plan.TwilightThreshold(plan.TwilightKind(200)); ok {
		t.Error("TwilightThreshold reports an invented kind as defined")
	}
}
