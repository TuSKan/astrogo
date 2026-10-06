package remote

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

// appName is the directory name under the OS user cache dir holding all
// astrogo data by default.
const appName = "astrogo"

// DataDirEnv overrides the default data location when SetDataDir has not
// been called. Its value is a filesystem URL, not an OS path — see DataDirURL.
const DataDirEnv = "ASTROGO_CACHE_DIR"

// SetDataDir sets the base location for all data astrogo stores, as a
// filesystem URL [OpenFS] can open and write to:
// "file:///home/u/.cache/astrogo?create_dir=true", or "mem://scratch" for a
// cache that never lands. Nothing astrogo caches is assumed to live on local
// disk, so a backend for a further scheme would serve here unchanged; [Schemes]
// lists the ones registered.
func SetDataDir(fsURL string) { Default().SetDataDir(fsURL) }

// SetDataDir sets the base location for everything this client stores. See the
// package-level [SetDataDir].
func (c *Client) SetDataDir(fsURL string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.dataDirURL = fsURL
}

// DataDirURL returns the filesystem URL astrogo stores all its data under,
// resolved in order: an explicit SetDataDir call, then DataDirEnv, then
// the OS user cache directory — ~/.cache/astrogo on Linux,
// %LocalAppData%\astrogo on Windows, ~/Library/Caches/astrogo on macOS.
// Re-resolved per call, so a changed environment takes effect immediately.
func DataDirURL() string { return Default().DataDirURL() }

// DataDirURL returns the filesystem URL this client stores its data under,
// resolved in order: an explicit SetDataDir call on this client, then
// DataDirEnv, then the OS user cache directory. Re-resolved per call, so a
// changed environment takes effect immediately.
func (c *Client) DataDirURL() string {
	c.mu.RLock()

	d := c.dataDirURL

	c.mu.RUnlock()

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
// The result carries create_dir=true because the file:// opener creates its
// directory only when asked — a read-only source must not have one made — so
// a first run would otherwise fail to open a cache directory that does not
// exist yet. It is built through url.URL rather than concatenation: a '#' in
// the path would silently truncate it and swallow the query, and a stray '%'
// would make it unparseable.
//
// Writes stage inside the cache directory and are renamed into place, under
// names unique by process id and counter, so a multi-gigabyte kernel never
// crosses a volume and concurrent writers of one key cannot collide (#315).
// The no_tmp_dir=1 parameter this comment used to weigh was gocloud's
// fileblob's, and the file:// backend ignores it.
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

// DataDir opens DataDirURL as an [FS] rooted at astrogo's base data
// location.
func DataDir(ctx context.Context) (FS, error) { return Default().DataDir(ctx) }

// DataDir opens this client's [Client.DataDirURL] as an [FS].
func (c *Client) DataDir(ctx context.Context) (FS, error) {
	fsys, err := OpenFS(ctx, c.DataDirURL())
	if err != nil {
		return nil, fmt.Errorf("remote: open data dir: %w", err)
	}

	return fsys, nil
}

// CacheDir returns the [FS] and key prefix an endpoint caches under. It
// creates nothing: a "directory" is only a key prefix, so the first
// write under it is all the backend needs. Returns ErrUnknownEndpoint for an
// unregistered id.
//
// Every registered endpoint has one, KindAPI included. A cache directory is
// somewhere to put bytes, which is a different question from whether GetFile
// can fetch them: GetFile needs a filesystem URL and a name and so still requires
// KindFile, but a decoded API payload is content this module is expected to
// keep - [WriteFile] exists for exactly that - and it needs a place to go.
//
// This used to refuse KindAPI, which made that impossible and quietly
// disabled the callers that had already been written for it. starlight asks
// for esa.gaia's cache directory to checkpoint an hour-long Gaia aggregation
// and resume it across sessions; both its read and its write sat behind a
// "cache directory was available" branch that could never be taken, so the
// aggregation restarted from nothing every time. Nothing reported it, because
// a cache that cannot be reached is indistinguishable from a cold one.
func CacheDir(ctx context.Context, id EndpointID) (fsys FS, prefix string, err error) {
	return Default().CacheDir(ctx, id)
}

// CacheDir returns the [FS] and key prefix an endpoint caches under for this
// client. See the package-level [CacheDir].
func (c *Client) CacheDir(ctx context.Context, id EndpointID) (fsys FS, prefix string, err error) {
	ep, ok := c.Lookup(id)
	if !ok {
		return nil, "", fmt.Errorf("%w: %q", ErrUnknownEndpoint, id)
	}

	fsys, err = c.DataDir(ctx)
	if err != nil {
		return nil, "", err
	}

	return fsys, ep.Subsystem + "/", nil
}
