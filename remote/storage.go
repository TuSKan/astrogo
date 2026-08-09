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

// DataDirEnv is the environment variable that overrides the default
// astrogo data directory when SetDataDir/SetDataDirPath hasn't been
// called explicitly — see DataDir's precedence order. Useful for CI and
// containers that want a fixed cache location without touching
// application code.
const DataDirEnv = "ASTROGO_CACHE_DIR"

// DataDir returns the base directory for all astrogo data, resolved in
// this order: an explicit SetDataDir/SetDataDirPath call, then the
// DataDirEnv environment variable, then os.UserCacheDir()/astrogo
// (falling back to os.TempDir() when the user cache dir is unavailable):
// ~/.cache/astrogo on Linux, %LocalAppData%\astrogo on Windows,
// ~/Library/Caches/astrogo on macOS. Re-resolved on every call — there is
// nothing to invalidate if the environment or an explicit override
// changes between calls.
func DataDir() gofs.File {
	dataMu.RLock()

	d := dataDir

	dataMu.RUnlock()

	if d != "" {
		return d
	}

	if env := os.Getenv(DataDirEnv); env != "" {
		return gofs.File(env)
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

// CacheDir returns the on-disk cache directory for a file-bearing endpoint
// (KindFile, KindS3), creating it if needed. Returns ErrUnknownEndpoint
// for an unregistered id or an error for a KindAPI endpoint, which has no
// cache directory.
func CacheDir(id EndpointID) (gofs.File, error) {
	ep, ok := Lookup(id)
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrUnknownEndpoint, id)
	}

	if !ep.Kind.cacheable() {
		return "", fmt.Errorf("%w: %q has no cache directory", ErrNotFileEndpoint, id)
	}

	return subsystemDir(ep.Subsystem)
}
