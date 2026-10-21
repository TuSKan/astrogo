package openngc

import (
	"strings"
	"testing"
)

// FuzzParseOpenNGC holds the OpenNGC CSV reader to never panicking or hanging
// on whatever the catalog's repository serves (#746). Rows are
// semicolon-separated, with sexagesimal positions split on colons.
func FuzzParseOpenNGC(f *testing.F) {
	f.Add(realNGCRows)
	f.Add(realAddendumRows)
	f.Add("Name;Type;RA;Dec\nX;*;19:25;+20:11:00\n")
	f.Add("")

	f.Fuzz(func(_ *testing.T, s string) {
		_, _ = parseOpenNGC(strings.NewReader(s))
	})
}
