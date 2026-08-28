package remote

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"

	"github.com/TuSKan/astrogo/remote/file"
)

// appName is the directory name under the OS user cache dir holding all
// astrogo data by default.
const appName = "astrogo"

// dataDirURL is the process-wide base location for everything astrogo
// stores. Empty means "resolve the default lazily" — see DataDirURL.
//
//nolint:gochecknoglobals // process-wide data location is this package's purpose
var (
	dataMu     sync.RWMutex
	dataDirURL string
)

// DataDirEnv overrides the default data location when SetDataDir has not
// been called. Its value is a bucket URL, not an OS path — see DataDirURL.
const DataDirEnv = "ASTROGO_CACHE_DIR"

// SetDataDir sets the base location for all data astrogo stores, as any
// URL remote/file can open: "file:///home/u/.cache/astrogo?create_dir=true",
// "s3://my-cache-bucket", "sftp://host/path". Nothing astrogo caches is
// assumed to live on local disk.
func SetDataDir(bucketURL string) {
	dataMu.Lock()
	defer dataMu.Unlock()

	dataDirURL = bucketURL
}

// DataDirURL returns the bucket URL astrogo stores all its data under,
// resolved in order: an explicit SetDataDir call, then DataDirEnv, then
// the OS user cache directory — ~/.cache/astrogo on Linux,
// %LocalAppData%\astrogo on Windows, ~/Library/Caches/astrogo on macOS.
// Re-resolved per call, so a changed environment takes effect immediately.
func DataDirURL() string {
	dataMu.RLock()

	d := dataDirURL

	dataMu.RUnlock()

	if d != "" {
		return d
	}

	if env := os.Getenv(DataDirEnv); env != "" {
		return env
	}

	return defaultDataDirURL()
}

// defaultDataDirURL is the one place in astrogo that converts an OS
// filesystem path into a URL. It exists because the default location can
// only come from os.UserCacheDir; every other path into this package is a
// URL supplied by the caller. It is deliberately unexported — a general
// path-to-URL helper would invite call sites that assume local disk.
//
// The result carries create_dir=true because fileblob's URL opener
// defaults CreateDir to false, so a first run would otherwise fail to open
// a cache directory that does not exist yet. It is built through url.URL
// rather than concatenation: a '#' in the path would silently truncate it
// and swallow the query, and a stray '%' would make it unparseable.
func defaultDataDirURL() string {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}

	slash := filepath.ToSlash(filepath.Join(base, appName))
	if slash == "" || slash[0] != '/' {
		slash = "/" + slash // Windows drive-letter paths are not "/"-rooted
	}

	u := url.URL{Scheme: "file", Path: slash, RawQuery: "create_dir=true"}

	return u.String()
}

// DataDir opens DataDirURL as a Bucket rooted at astrogo's base data
// location.
func DataDir(ctx context.Context) (*file.Bucket, error) {
	b, err := file.Open(ctx, DataDirURL())
	if err != nil {
		return nil, fmt.Errorf("remote: open data dir: %w", err)
	}

	return b, nil
}

// CacheDir returns the Bucket and key prefix an endpoint caches under. It
// creates nothing: a bucket "directory" is only a key prefix, so the first
// write under it is all the backend needs. Returns ErrUnknownEndpoint for an
// unregistered id.
//
// Every registered endpoint has one, KindAPI included. A cache directory is
// somewhere to put bytes, which is a different question from whether GetFile
// can fetch them: GetFile needs a bucket URL and a name and so still requires
// KindFile, but a decoded API payload is content this module is expected to
// keep - file.Save exists for exactly that - and it needs a place to go.
//
// This used to refuse KindAPI, which made that impossible and quietly
// disabled the callers that had already been written for it. starlight asks
// for esa.gaia's cache directory to checkpoint an hour-long Gaia aggregation
// and resume it across sessions; both its read and its write sat behind a
// "cache directory was available" branch that could never be taken, so the
// aggregation restarted from nothing every time. Nothing reported it, because
// a cache that cannot be reached is indistinguishable from a cold one.
func CacheDir(ctx context.Context, id EndpointID) (bucket *file.Bucket, prefix string, err error) {
	ep, ok := Lookup(id)
	if !ok {
		return nil, "", fmt.Errorf("%w: %q", ErrUnknownEndpoint, id)
	}

	bucket, err = DataDir(ctx)
	if err != nil {
		return nil, "", err
	}

	return bucket, ep.Subsystem + "/", nil
}
