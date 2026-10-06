package time_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

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

// TestFSEOPLoaderTellsAbsenceFromFailure holds the distinction the old
// FileEOPLoader erased: it returned ErrNoEOPData for any read error, so a
// bulletin that was there and could not be read looked exactly like one that
// had never been seeded.
func TestFSEOPLoaderTellsAbsenceFromFailure(t *testing.T) {
	t.Parallel()

	t.Run("missing is ErrNoEOPData", func(t *testing.T) {
		t.Parallel()

		loader := time.FSEOPLoader{FS: fstest.MapFS{}, Name: "finals2000A.data"}

		// "Nothing pre-seeded here" is exactly what a fresh deployment looks
		// like, so it is an ordinary state rather than a failure.
		if _, err := loader.Cached(t.Context()); !errors.Is(err, time.ErrNoEOPData) {
			t.Errorf("Cached on a missing object returned %v, want ErrNoEOPData", err)
		}
	})

	t.Run("unreadable is itself", func(t *testing.T) {
		t.Parallel()

		loader := time.FSEOPLoader{FS: deniedFS{}, Name: "finals2000A.data"}

		_, err := loader.Cached(t.Context())
		if !errors.Is(err, fs.ErrPermission) {
			t.Errorf("Cached on an unreadable object returned %v, want one wrapping fs.ErrPermission", err)
		}

		if errors.Is(err, time.ErrNoEOPData) {
			t.Errorf("Cached on an unreadable object returned %v, which reads as \"nothing seeded\"", err)
		}
	})

	t.Run("zero value is a misconfiguration", func(t *testing.T) {
		t.Parallel()

		// A nil FS used to be impossible to express and would panic inside
		// fs.ReadFile; it is reported instead, and not as absence.
		_, err := time.FSEOPLoader{}.Cached(t.Context())
		if err == nil || errors.Is(err, time.ErrNoEOPData) {
			t.Errorf("Cached on a zero FSEOPLoader returned %v, want an error that is not ErrNoEOPData", err)
		}
	})
}

// deniedFS refuses every open with fs.ErrPermission: a bulletin that is there
// and cannot be read.
type deniedFS struct{}

func (deniedFS) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
}
