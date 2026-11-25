package remotetest

import (
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/TuSKan/astrogo/remote"
)

// TestRunWithoutAnEmptyCacheRunsNothing: when the untagged build cannot make
// its empty cache, Run fails without running the tests, which would otherwise
// run online against whatever this machine has cached.
func TestRunWithoutAnEmptyCacheRunsNothing(t *testing.T) {
	if tiered {
		t.Skip("a tagged build makes no cache to fail at: Run only runs the tests")
	}

	t.Cleanup(remote.Capture().Restore)

	// The temporary directory every platform reads, pointed somewhere that
	// does not exist; os.MkdirTemp creates no parents.
	missing := filepath.Join(t.TempDir(), "missing", "deeper")
	for _, v := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(v, missing)
	}

	ran := false

	if code := Run(func() int { ran = true; return 0 }); code != 1 {
		t.Errorf("Run returned %d without its empty cache, want 1", code)
	}

	if ran {
		t.Error("Run ran the tests without its empty cache")
	}
}

// TestRunGivesTheUntaggedBuildAnEmptyCacheOffline: in the untagged build, Run
// hands run an offline client with a cache directory of its own, empty, and
// removes that directory afterward; in a tagged build it changes nothing.
// Either way it returns run's exit code.
func TestRunGivesTheUntaggedBuildAnEmptyCacheOffline(t *testing.T) {
	t.Cleanup(remote.Capture().Restore)
	remote.SetOffline(false)

	before := remote.DataDirURL()

	var (
		offline bool
		dirURL  string
		entries = -1
		fsys    remote.FS
	)

	code := Run(func() int {
		offline = remote.Offline()
		dirURL = remote.DataDirURL()

		var err error

		fsys, err = remote.DataDir(t.Context())
		if err != nil {
			t.Errorf("DataDir inside Run: %v", err)

			return 0
		}

		list, err := fs.ReadDir(fsys, ".")
		if err != nil {
			t.Errorf("read the cache inside Run: %v", err)

			return 0
		}

		entries = len(list)

		return 7
	})

	if code != 7 {
		t.Errorf("Run returned %d, want run's 7", code)
	}

	if tiered {
		if offline || dirURL != before {
			t.Errorf("a tagged build ran offline=%v with the cache at %q, want the client as it was (%q)",
				offline, dirURL, before)
		}

		return
	}

	if !offline {
		t.Error("the untagged build ran online")
	}

	if dirURL == before {
		t.Errorf("the untagged build ran with the cache it found, %q", before)
	}

	if entries != 0 {
		t.Errorf("the untagged build's cache held %d entries, want none", entries)
	}

	if fsys != nil {
		if _, err := fs.ReadDir(fsys, "."); err == nil {
			t.Error("the untagged build's cache directory is still there after Run")
		}
	}
}
