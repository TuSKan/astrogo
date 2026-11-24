package plan

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/epoch"
)

// detailValue returns the value on the line of s that starts with label,
// with the padding between them removed, or "" if no line does.
func detailValue(s, label string) string {
	for line := range strings.SplitSeq(s, "\n") {
		if rest, ok := strings.CutPrefix(line, label); ok {
			return strings.TrimLeft(rest, " ")
		}
	}

	return ""
}

// TestStringAlignsItsValues: every value of a block starts in one column.
// The labels used to be followed by a tab, which a terminal expands to the
// next multiple of eight columns, so "Magnitude:" and "Radial velocity:" put
// their values eight columns apart, and the Sun's details in example
// 15_target_details printed ragged. The extra properties are their own block,
// aligned on their own labels.
func TestStringAlignsItsValues(t *testing.T) {
	t.Parallel()

	rise := epoch.Date(2026, time.April, 25, 9, 20, 0, 0, time.UTC)
	d := TargetDetails{
		Name:           "Sun",
		Description:    "Our local star",
		Source:         "JPL DE442",
		Magnitude:      "-26.7 mag",
		RadialVelocity: "+0.85 km/s topocentric",
		RA:             angle.Deg(32.97),
		Dec:            angle.Deg(13.27),
		AngularSize:    "31.78′",
		RiseTime:       &rise,
		TransitTime:    &rise,
		SetTime:        &rise,
		ExtraProps:     map[string]string{"Messier number": "M 31", "Spectral type": "G2V"},
	}

	s := d.String()
	if strings.Contains(s, "\t") {
		t.Fatalf("String() still separates with a tab:\n%s", s)
	}

	blocks := strings.Split(s, "\n\n")
	last := blocks[len(blocks)-2:] // the known fields, then the extra properties

	for i, block := range last {
		col := -1

		for line := range strings.SplitSeq(strings.TrimSpace(block), "\n") {
			label, rest, ok := strings.Cut(line, ":")
			if !ok {
				t.Fatalf("block %d: line %q has no label", i, line)
			}

			// The value's column, in characters: the label, the colon, and the
			// padding before the value.
			c := utf8.RuneCountInString(label) + 1 + len(rest) - len(strings.TrimLeft(rest, " "))
			if col == -1 {
				col = c
			} else if c != col {
				t.Errorf("block %d: %q's value starts at column %d, the block's first at %d:\n%s", i, label, c, col, s)
			}
		}
	}

	if got := detailValue(s, "Radial velocity:"); got != "+0.85 km/s topocentric" {
		t.Errorf("the radial velocity line reads %q", got)
	}

	if got := detailValue(s, "Spectral type:"); got != "G2V" {
		t.Errorf("the extra property reads %q", got)
	}
}
