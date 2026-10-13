package vizier

import (
	"strings"
	"testing"
)

// FuzzParseCSV holds VizieR's CSV reader to never panicking or hanging on
// whatever a TAP answer holds, against each table schema the provider knows
// (#746). The reader finds its columns by header name and indexes each row
// by them.
func FuzzParseCSV(f *testing.F) {
	f.Add("designation,ra,dec\n32349,101.28,-16.71\n")
	f.Add("ra,dec\n1,2\n")
	f.Add("")

	f.Fuzz(func(_ *testing.T, s string) {
		for _, schema := range tableSchemas {
			_, _ = parseCSV(strings.NewReader(s), schema)
		}
	})
}
