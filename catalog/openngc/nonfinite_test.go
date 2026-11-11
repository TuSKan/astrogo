package openngc

import (
	"strings"
	"testing"
)

// A NaN or infinite magnitude in the OpenNGC CSV is no magnitude (#1080).
//
// strconv.ParseFloat accepts both, and a NaN V-Mag became a target with
// HasVMag set and a magnitude that failed every brightness cut, so the object
// dropped out of "what is visible tonight" as though it were too faint.
func TestOpenNGCReadsANonFiniteMagnitudeAsAbsent(t *testing.T) {
	t.Parallel()

	for _, bad := range []string{"NaN", "Inf", "-Infinity"} {
		// IC4715's V-Mag is 4.50 in the real rows.
		body := strings.Replace(realNGCRows, "90;;4.50;", "90;;"+bad+";", 1)
		if body == realNGCRows {
			t.Fatal("the fixture no longer holds IC4715's V-Mag where this test expects it")
		}

		records, err := parseOpenNGC(strings.NewReader(body))
		if err != nil {
			t.Fatalf("%s: parseOpenNGC: %v", bad, err)
		}

		found := false

		for _, tgt := range toTargets(records) {
			switch tgt.ID {
			case "IC4715":
				found = true

				if tgt.HasVMag {
					t.Errorf("V-Mag %q: HasVMag is true with VMag %v", bad, tgt.VMag)
				}

				if !tgt.HasCoord {
					t.Errorf("V-Mag %q cost IC4715 its position", bad)
				}
			case "NGC1936":
				if !tgt.HasVMag || tgt.VMag != 11.60 {
					t.Errorf("the control row's V-Mag 11.60 was not read: %v, %t", tgt.VMag, tgt.HasVMag)
				}
			}
		}

		if !found {
			t.Fatalf("V-Mag %q: IC4715 is missing", bad)
		}
	}
}
