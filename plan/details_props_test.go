package plan

import (
	"slices"
	"strings"
	"testing"
)

// TestStringPrintsExtraPropsInAStableOrder: past the Messier and NGC/IC
// numbers, String ranged over the ExtraProps map, so a target with several
// other properties printed them in a different order from run to run
// (#1134). They now follow the two fixed ones in key order.
func TestStringPrintsExtraPropsInAStableOrder(t *testing.T) {
	d := TargetDetails{
		Name: "M45",
		ExtraProps: map[string]string{
			"Spectral type":       "B7IIIe",
			"Proper motion (RA)":  "19.997 mas/yr",
			"Proper motion (Dec)": "-43.077 mas/yr",
			"NGC/IC number":       "none",
			"Messier number":      "M45",
			"Constellation":       "Taurus",
			"Aliases":             "Pleiades",
		},
	}

	want := []string{
		"Messier number", "NGC/IC number",
		"Aliases", "Constellation", "Proper motion (Dec)", "Proper motion (RA)", "Spectral type",
	}

	first := d.String()

	var got []string

	for line := range strings.Lines(first) {
		if key, _, ok := strings.Cut(line, ":\t"); ok && d.ExtraProps[key] != "" {
			got = append(got, key)
		}
	}

	if !slices.Equal(got, want) {
		t.Errorf("ExtraProps printed in order %q, want %q", got, want)
	}

	// Go randomizes map iteration on every range, so twenty more renderings
	// would almost surely have shown a second order before the fix.
	for range 20 {
		if s := d.String(); s != first {
			t.Fatalf("String() changed between calls:\n%s\n---\n%s", first, s)
		}
	}
}
