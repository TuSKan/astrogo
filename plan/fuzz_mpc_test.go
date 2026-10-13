package plan

import (
	"strings"
	"testing"
)

// FuzzParseMPCObsCodes holds the MPC observatory-code reader to never
// panicking or hanging on whatever the Minor Planet Center serves (#746). The
// file is fixed-column, and some rows run their fields together with no
// separator, so every field is sliced by column.
func FuzzParseMPCObsCodes(f *testing.F) {
	f.Add(mpcFixture)
	f.Add("<pre>\nCode  Long.   cos      sin    Name\n005   2.231000.659891\n</pre>")
	f.Add("")

	f.Fuzz(func(_ *testing.T, s string) {
		_, _ = parseMPCObsCodes(strings.NewReader(s))
	})
}
