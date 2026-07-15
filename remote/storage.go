package remote

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	gofs "github.com/ungerik/go-fs"
)

// appName is the directory name under the OS user cache dir that holds all
// astrogo data by default.
const appName = "astrogo"

// dataDir holds the process-wide base location for ALL data astrogo stores
// (JPL SPK/LSK kernels, the IERS EOP cache). It is a go-fs File — a
// URI-style path backed by a pluggable filesystem registry — so a future
// blob/bucket backend (s3://, gs://) can be plugged in by registering its
// scheme with github.com/ungerik/go-fs and calling SetDataDir; astrogo call
// sites don't change.
//
//nolint:gochecknoglobals // process-wide data location is this package's purpose
var (
	dataMu  sync.RWMutex
	dataDir gofs.File // empty = resolve default lazily
)

// SetDataDir sets the base directory for all data astrogo stores. Accepts
// any go-fs File, including ones on filesystems registered under non-local
// schemes.
func SetDataDir(dir gofs.File) {
	dataMu.Lock()
	defer dataMu.Unlock()

	dataDir = dir
}

// SetDataDirPath is the local-path convenience form of SetDataDir.
func SetDataDirPath(path string) {
	SetDataDir(gofs.File(path))
}

// DataDir returns the base directory for all astrogo data. Unless
// overridden via SetDataDir, it defaults to os.UserCacheDir()/astrogo
// (falling back to os.TempDir() when the user cache dir is unavailable):
// ~/.cache/astrogo on Linux, %LocalAppData%\astrogo on Windows,
// ~/Library/Caches/astrogo on macOS.
func DataDir() gofs.File {
	dataMu.RLock()

	d := dataDir

	dataMu.RUnlock()

	if d != "" {
		return d
	}

	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}

	return gofs.File(filepath.Join(base, appName))
}

// subsystemDir returns DataDir()/<subsystem> (e.g. "jpl", "iers"), creating
// it if it does not yet exist.
func subsystemDir(subsystem string) (gofs.File, error) {
	dir := DataDir().Join(subsystem)

	if err := dir.MakeAllDirs(); err != nil {
		return dir, fmt.Errorf("remote: mkdir %s: %w", dir, err)
	}

	return dir, nil
}

// CacheDir returns the on-disk cache directory for a KindFile endpoint,
// creating it if needed. Returns ErrUnknownEndpoint for an unregistered id
// or an error if id is a KindAPI endpoint (which has no cache directory).
func CacheDir(id EndpointID) (gofs.File, error) {
	ep, ok := Lookup(id)
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrUnknownEndpoint, id)
	}

	if ep.Kind != KindFile {
		return "", fmt.Errorf("%w: %q has no cache directory", ErrNotFileEndpoint, id)
	}

	return subsystemDir(ep.Subsystem)
}
