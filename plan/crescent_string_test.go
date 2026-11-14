package plan

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestCrescentStringAlignsEveryCode: in CrescentResult.String every criterion
// row puts its ARCV column at the same place, whatever verdict or zone code it
// carries, and a zone row names its code once. The column was twelve
// characters wide, so Odeh's "Optical/Naked" pushed its row one character out
// of line, and every zone row printed its code twice (#1167).
func TestCrescentStringAlignsEveryCode(t *testing.T) {
	t.Parallel()

	for _, odeh := range []string{"Naked Eye", "Optical/Naked", "Optical Only", "Not Visible"} {
		r := CrescentResult{
			Fotheringham: CrescentVerdict{Visible: true},
			Yallop:       CrescentZone{Code: "F", Label: "Not visible"},
			Odeh:         CrescentZone{Code: odeh, Label: "label"},
			Qureshi:      CrescentZone{Code: "A", Label: "Easily visible"},
		}

		col := -1

		for line := range strings.SplitSeq(r.String(), "\n") {
			before, _, found := strings.Cut(line, "ARCV")
			if !found {
				continue
			}

			// Columns, not bytes: the rows hold degree and prime signs.
			at := utf8.RuneCountInString(before)
			if col < 0 {
				col = at
			} else if at != col {
				t.Errorf("Odeh %q: ARCV at column %d in %q, %d in the rows before", odeh, at, line, col)
			}

			if strings.HasPrefix(strings.TrimSpace(line), "Odeh") && strings.Count(line, odeh) != 1 {
				t.Errorf("Odeh %q: the zone row names its code %d times: %q", odeh, strings.Count(line, odeh), line)
			}
		}

		if col < 0 {
			t.Fatalf("Odeh %q: String printed no criterion rows:\n%s", odeh, r.String())
		}
	}
}
