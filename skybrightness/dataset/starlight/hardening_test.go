package starlight

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestLoadRefusesAHugePixelIndexWithoutAllocatingForIt (#746): a twelve-pixel
// map with one row numbered 200000000000000 is out of range, and says so.
// Until #746 the duplicate check sized a slice by the largest index in the
// file, so this row asked for 200 TB and killed the process before any error
// could be returned; a fuzzer found it.
func TestLoadRefusesAHugePixelIndexWithoutAllocatingForIt(t *testing.T) {
	t.Parallel()

	var b strings.Builder

	b.WriteString("# bands: V\n200000000000000 0.5\n")

	for pixel := 1; pixel < 12; pixel++ {
		fmt.Fprintf(&b, "%d 1.0\n", pixel)
	}

	if _, err := Load(strings.NewReader(b.String()), Galactic); !errors.Is(err, ErrPixelRange) {
		t.Errorf("got %v, want ErrPixelRange", err)
	}
}

// TestBandHeaderKeepsCasingAndSurvivesAnyBytes (#746): the "bands:" prefix is
// matched whatever its case and the names keep theirs. Until #746 the line was
// lowercased and the original sliced by the lowercase's length, which panicked
// whenever the two lengths differed — invalid UTF-8 lowercases to the
// three-byte U+FFFD — as a fuzzer found.
func TestBandHeaderKeepsCasingAndSurvivesAnyBytes(t *testing.T) {
	t.Parallel()

	got, ok := bandHeader("# BANDS: Gaia_G, V")
	if !ok || len(got) != 2 || got[0] != "Gaia_G" || got[1] != "V" {
		t.Errorf("bandHeader kept %v (%v), want [Gaia_G V]", got, ok)
	}

	for _, line := range []string{"#BAnds:\xdf\xdf\xdf\xdf", "# bands:", "# band", "#"} {
		if names, ok := bandHeader(line); ok && len(names) == 0 {
			t.Errorf("%q: accepted with no names", line)
		}
	}
}
