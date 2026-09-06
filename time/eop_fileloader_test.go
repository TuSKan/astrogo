package time_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/TuSKan/astrogo/time"
)

// TestFileEOPLoader covers the pre-seeded, no-dependencies EOP path.
//
// It had no test, no example and no caller anywhere in the module — found by
// indexing every exported declaration against every reference (#106). That
// matters more here than the count suggests: FileEOPLoader is the documented
// answer for a deployment that wants EOP data without pulling in
// cloud-storage and gRPC machinery, so it is a path recommended to users and
// exercised by nobody.
func TestFileEOPLoader(t *testing.T) {
	t.Parallel()

	// One real finals2000A record. The columns are fixed-width and the parser
	// slices by index, so this is copied from the bulletin's own format rather
	// than approximated.
	const record = "73 1 2 41684.00 I  0.120733 0.009786  0.136966 0.015902  " +
		"I 0.8084178 0.0002710  0.0000 0.1916  P    -0.766    0.199    -0.720    0.300"

	path := filepath.Join(t.TempDir(), "finals2000A.data")
	if err := os.WriteFile(path, []byte(record+"\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	loader := time.FileEOPLoader(path)

	data, err := loader.Cached(t.Context())
	if err != nil {
		t.Fatalf("Cached: %v", err)
	}

	if len(data.Raw) == 0 {
		t.Error("Cached returned no bytes for a file that exists")
	}

	if data.ModTime.IsZero() {
		t.Error("Cached returned a zero ModTime; the staleness check has nothing to compare")
	}

	// Fetch must report absence rather than reaching for the network — that
	// promise is the whole reason this type exists.
	if _, err := loader.Fetch(t.Context()); !errors.Is(err, time.ErrNoEOPData) {
		t.Errorf("Fetch returned %v, want ErrNoEOPData.\n"+
			"  A FileLoader downloads nothing; returning anything else here would mean the "+
			"no-dependencies path had grown one.", err)
	}

	// A missing file is an ordinary state, not a failure: "nothing pre-seeded
	// here" is exactly what a fresh deployment looks like.
	missing := time.FileEOPLoader(filepath.Join(t.TempDir(), "absent.data"))
	if _, err := missing.Cached(t.Context()); !errors.Is(err, time.ErrNoEOPData) {
		t.Errorf("Cached on a missing file returned %v, want ErrNoEOPData", err)
	}
}
