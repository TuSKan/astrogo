package passband_test

import (
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/skybrightness/dataset/passband"
)

// FuzzParse holds the SVO filter-profile reader to never panicking or hanging
// on whatever an SVO response holds (#746). The bytes are the Spanish Virtual
// Observatory's, and the reader takes its metadata from PARAMs it looks up by
// name and its curve from a table it trusts to be two columns wide.
func FuzzParse(f *testing.F) {
	f.Add(profile(goodParams(), goodRows()))
	f.Add(profileResourceLevel(goodParams(), goodRows()))
	f.Add(profile(goodParams(), nil))
	f.Add("")

	f.Fuzz(func(_ *testing.T, s string) {
		_, _ = passband.Parse(strings.NewReader(s))
	})
}
