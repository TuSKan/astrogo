package mpcorb_test

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/catalog/mpcorb"
)

// FuzzRead holds the MPC orbit-file reader to never panicking or hanging on
// whatever bytes it is handed (#746). They are the Minor Planet Center's —
// MPCORB.DAT alone runs to 317 MB, maintained by many hands — or a local copy
// of any provenance, and every row is sliced by fixed column, which is where
// an off-by-one or a short row would surface.
//
// Seeded with the package's own fixtures: asteroid rows covering a packed
// number above 620000 and a blank magnitude block, the comet rows covering
// fragments, an open orbit and a backtick in a name, and a gzip stream, since
// Read sniffs the magic bytes rather than trusting a name.
func FuzzRead(f *testing.F) {
	f.Add([]byte(fixture))
	f.Add([]byte(strings.Join(cometRows, "\n")))
	f.Add([]byte(fixture + strings.Join(cometRows, "\n")))
	f.Add([]byte{0x1f, 0x8b})
	f.Add([]byte{})

	var gz bytes.Buffer

	w := gzip.NewWriter(&gz)
	_, _ = w.Write([]byte(fixture))
	_ = w.Close()

	f.Add(gz.Bytes())

	f.Fuzz(func(_ *testing.T, b []byte) {
		for tgt, err := range mpcorb.Read(bytes.NewReader(b)) {
			_, _ = tgt, err
		}
	})
}
