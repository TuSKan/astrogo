package dust

import "testing"

// FuzzParse holds the IRSA dust-service reader to never panicking or hanging
// on whatever IRSA answers (#746). It picks the 100 micron block out of an
// XML document by regular expression and reads the value it captures.
func FuzzParse(f *testing.F) {
	f.Add(sample)
	f.Add(`<results><result><desc>100 micron</desc></result></results>`)
	f.Add("")

	f.Fuzz(func(_ *testing.T, doc string) {
		_, _ = parse(doc)
	})
}
