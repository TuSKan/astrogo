package time_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/TuSKan/astrogo/time"
)

// finalsRecord is one real finals2000A record. The columns are fixed-width and
// the parser slices by index, so this is copied from the bulletin's own format
// rather than approximated.
const finalsRecord = "73 1 2 41684.00 I  0.120733 0.009786  0.136966 0.015902  " +
	"I 0.8084178 0.0002710  0.0000 0.1916  P    -0.766    0.199    -0.720    0.300"

// TestFSEOPLoaderReadsADirectoryOnDisk covers the use the doc comment shows:
// a pre-seeded bulletin in a directory, through os.DirFS, written at the
// caller's call site so that astrogo's own API never takes an OS path (#509).
//
// How the loader tells a missing bulletin from an unreadable one is tested
// beside it, in time/internal/iers.
func TestFSEOPLoaderReadsADirectoryOnDisk(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "finals2000A.data"), []byte(finalsRecord+"\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	loader := time.FSEOPLoader{FS: os.DirFS(dir), Name: "finals2000A.data"}

	data, err := loader.Cached(t.Context())
	if err != nil {
		t.Fatalf("Cached: %v", err)
	}

	if string(data.Raw) != finalsRecord+"\n" {
		t.Errorf("Cached returned %q, want the bulletin as written", data.Raw)
	}

	if data.ModTime.IsZero() {
		t.Error("Cached returned a zero ModTime; the staleness check has nothing to compare")
	}

	// Fetch must report absence rather than reaching for the network — that
	// promise is the whole reason this type exists.
	if _, err := loader.Fetch(t.Context()); !errors.Is(err, time.ErrNoEOPData) {
		t.Errorf("Fetch returned %v, want ErrNoEOPData.\n"+
			"  An FSEOPLoader downloads nothing; returning anything else here would mean the "+
			"no-dependencies path had grown one.", err)
	}
}
