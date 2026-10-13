package jpl

import (
	"os"
	"testing"
)

// FuzzParseHorizons holds the three Horizons response readers to never
// panicking or hanging on whatever the service answers (#746). The tables are
// cut at column offsets taken from a separator line of dashes, and the cells
// are then read by position.
//
// Seeded with real Horizons responses from testdata, one of each shape: an
// exact match, a major-body table and a small-body index.
func FuzzParseHorizons(f *testing.F) {
	for _, name := range []string{"titan.txt", "iss.txt", "halley.txt", "exact-bennu.txt"} {
		b, err := os.ReadFile("testdata/" + name)
		if err != nil {
			f.Fatalf("seed %s: %v", name, err)
		}

		f.Add(string(b))
	}

	f.Add("-----  -----\n")
	f.Add("")

	f.Fuzz(func(_ *testing.T, data string) {
		_, _ = parseExactMatch(data)
		_, _ = parseMajorBodyMatchTable(data)
		_, _ = parseSmallBodyIndexTable(data)
	})
}
