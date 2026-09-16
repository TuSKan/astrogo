package remote

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"io/fs"

	"github.com/TuSKan/astrogo/logging"
	"github.com/TuSKan/astrogo/remote/file"
	"github.com/TuSKan/astrogo/time"
)

// readConfig carries per-GetFile options.
type readConfig struct {
	cacheName string
	validate  func(io.Reader) error
	timeout   time.Duration
	progress  func(downloaded, total int64)
}

// ReadOption customizes a single GetFile call.
type ReadOption func(*readConfig)

// WithCacheName sets the cache key when it differs from the source name —
// IERS serves finals2000A.all, which time caches as finals2000A.data.
func WithCacheName(key string) ReadOption {
	return func(c *readConfig) { c.cacheName = key }
}

// WithValidate runs f over the freshly downloaded bytes before they are
// promoted into the cache, so a corrupt fetch is never cached and never
// reused. f is not called for a cache hit. It reads a stream, not a
// buffer: a multi-GB kernel must not have to fit in memory to be checked.
func WithValidate(f func(io.Reader) error) ReadOption {
	return func(c *readConfig) { c.validate = f }
}

// WithDownloadTimeout overrides Endpoint.DownloadTimeout for one call.
func WithDownloadTimeout(d time.Duration) ReadOption {
	return func(c *readConfig) { c.timeout = d }
}

// WithProgress registers a callback invoked as a download progresses, with
// the bytes transferred so far and the total (0 if unknown). Never called
// for a cache hit.
func WithProgress(f func(downloaded, total int64)) ReadOption {
	return func(c *readConfig) { c.progress = f }
}

// GetFile ensures endpoint id's object named name is present and current
// in the cache, returning the cache [FS] and the key within it. The caller
// reads it however it needs — [Open], fs.ReadFile, or anything else written
// against fs.FS.
//
// An immutable endpoint's cache entry is reused on existence alone; a
// Mutable one is revalidated against the source's current ETag first. A
// miss downloads, which requires consent (ErrDownloadDenied otherwise) and
// is serialized against other processes doing the same.
func GetFile(ctx context.Context, id EndpointID, name string, opts ...ReadOption) (fsys FS, key string, err error) {
	return Default().GetFile(ctx, id, name, opts...)
}

// GetFile ensures endpoint id's object named name is present and current in
// this client's cache, returning that cache [FS] and the key within it. See
// the package-level [GetFile]; the consent, offline and URL decisions it makes
// are this client's, and the cache it fills is this client's.
func (c *Client) GetFile(ctx context.Context, id EndpointID, name string, opts ...ReadOption) (fsys FS, key string, err error) {
	ep, ok := c.Lookup(id)
	if !ok {
		return nil, "", fmt.Errorf("%w: %q", ErrUnknownEndpoint, id)
	}

	// URL is the offline/Disable gate. It runs first so a blocked endpoint
	// fails before any cache directory is resolved or lock taken, and so
	// the source below is never opened for a URL the caller may not reach.
	if _, err := c.URL(id); err != nil {
		return nil, "", err
	}

	if !ep.Kind.cacheable() {
		return nil, "", fmt.Errorf("%w: %q", ErrNotFileEndpoint, id)
	}

	var cfg readConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	cacheName := cfg.cacheName
	if cacheName == "" {
		cacheName = name
	}

	if cacheName == "" {
		return nil, "", fmt.Errorf("%w: endpoint %q", ErrCacheNameRequired, id)
	}

	cacheFS, prefix, err := c.CacheDir(ctx, id)
	if err != nil {
		return nil, "", err
	}

	cacheKey := prefix + cacheName

	// An immutable endpoint's cache hit is answered before the source is
	// ever resolved, so a fully-cached kernel stays readable even when its
	// source is unreachable — no network, no credentials, no driver.
	if !ep.Mutable {
		if exists, existsErr := file.Exists(ctx, cacheFS, cacheKey); existsErr == nil && exists {
			return cacheFS, cacheKey, nil
		}
	}

	srcFS, err := OpenFS(ctx, ep.URL)
	if err != nil {
		// A caller who never granted consent gets ErrDownloadDenied rather
		// than this source error even when the source is genuinely
		// unreachable: consent is the actionable blocker from their side,
		// and reporting it consistently matches the documented contract.
		// Routed through CheckDownload so a custom Policy still decides.
		// A caller who did grant consent sees the real error.
		if cerr := c.CheckDownload(id, name, ep.ApproxSize); cerr != nil {
			return nil, "", cerr
		}

		return nil, "", fmt.Errorf("remote: open source %s: %w", ep.URL, err)
	}

	if fresh, freshErr := freshInCache(ctx, ep, srcFS, cacheFS, name, cacheKey); freshErr == nil && fresh {
		return cacheFS, cacheKey, nil
	}

	// The lock spans the "still missing? then download" decision, not just
	// the transfer: without it two callers both observe the miss and both
	// write. go test runs each package as its own process and several
	// share one JPL kernel, so this is a cross-process race that no
	// in-process mutex can fix.
	release, err := file.AcquireLock(ctx, cacheFS, cacheKey)
	if err != nil {
		//nolint:wrapcheck // pure delegation to remote/file, internal to this package; its errors are already prefixed
		return nil, "", err
	}

	defer release()

	// Whoever held the lock before us may have filled the entry already.
	if fresh, freshErr := freshInCache(ctx, ep, srcFS, cacheFS, name, cacheKey); freshErr == nil && fresh {
		return cacheFS, cacheKey, nil
	}

	timeout := cmp.Or(cfg.timeout, ep.DownloadTimeout, DefaultDownloadTimeout)

	if err := c.fetchInto(ctx, id, ep, srcFS, cacheFS, name, cacheKey, timeout, cfg); err != nil {
		// A failed fetch is not the same as a missing file, and the difference
		// used to be a whole class of CI failure.
		//
		// This check was added because the lock above was exclusive within this
		// process and only mostly exclusive across processes: fileblob's
		// IfNotExist was a Stat followed by a Rename with a window in between,
		// so two processes could both reach here for one key, and the loser's
		// staging rename failed with "Access is denied" on Windows while the
		// winner's download completed perfectly. Measured in CI at the time:
		// `go test ./...` ran ephemeris/jpl and time as separate processes
		// against one cache, both fetched de440s.bsp, and one died on exactly
		// that rename while the other wrote a complete kernel (#241).
		//
		// Both halves of that are now gone — staging is named from the process
		// id rather than a clock, and the lock is O_CREATE|O_EXCL rather than
		// Stat-then-Rename — so this should no longer be reachable for that
		// reason. It is kept because it is not a workaround: it asks the
		// question the caller actually asked, is the object there and current,
		// and costs one Stat on a path that is already failing. It re-runs the
		// same freshness check the cache hit above uses and swallows nothing —
		// a fetch that failed for any other reason still finds nothing and
		// still fails.
		if fresh, freshErr := freshInCache(ctx, ep, srcFS, cacheFS, name, cacheKey); freshErr == nil && fresh {
			return cacheFS, cacheKey, nil
		}

		return nil, "", fmt.Errorf("remote: fetch %s: %w", name, err)
	}

	return cacheFS, cacheKey, nil
}

// Exists reports whether endpoint id currently serves an object at name.
// It is a metadata-only probe that transfers no body and so never triggers
// the consent gate — a caller may use it to discover what is available
// before deciding whether to ask for consent.
//
// A missing object is (false, nil): the source answered and it is not
// there. Any other failure returns an error, so "missing" is never
// confused with "could not tell".
func Exists(ctx context.Context, id EndpointID, name string) (bool, error) {
	return Default().Exists(ctx, id, name)
}

// Exists reports whether endpoint id currently serves an object at name, as
// this client may see it. See the package-level [Exists].
func (c *Client) Exists(ctx context.Context, id EndpointID, name string) (bool, error) {
	ep, ok := c.Lookup(id)
	if !ok {
		return false, fmt.Errorf("%w: %q", ErrUnknownEndpoint, id)
	}

	if _, err := c.URL(id); err != nil {
		return false, err
	}

	srcFS, err := OpenFS(ctx, ep.URL)
	if err != nil {
		return false, fmt.Errorf("remote: open source %s: %w", ep.URL, err)
	}

	//nolint:wrapcheck // pure delegation to remote/file, internal to this package; its errors are already prefixed
	return file.Exists(ctx, srcFS, name)
}

// freshInCache reports whether cacheKey already holds current content for
// ep+name, so GetFile can skip the transfer entirely.
func freshInCache(ctx context.Context, ep Endpoint, srcFS, cacheFS FS, name, cacheKey string) (bool, error) {
	exists, err := file.Exists(ctx, cacheFS, cacheKey)
	if err != nil {
		return false, fmt.Errorf("remote: check cache %s: %w", cacheKey, err)
	}

	if !exists {
		return false, nil
	}

	if !ep.Mutable {
		return true, nil
	}

	return unchanged(ctx, srcFS, cacheFS, name, cacheKey), nil
}

// unchanged compares the source ETag recorded beside the cached object at
// fetch time against the source's current one. Any failure — an erroring
// probe, offline mode — reports "changed", so GetFile falls through to its
// normal download path.
//
// The recorded ETag is deliberately not the cached object's own, and the reason
// survived the move off gocloud even though its mechanism did not. fileblob
// derived an object's ETag from its (ModTime, Size), which has nothing to do
// with the source's, so comparing those would never match and every reuse check
// would degrade into a full re-download. There is now no local ETag at all —
// [io/fs.FileInfo] has no such field — which makes the same point structurally:
// what is compared is what the source said, written down when the object was
// fetched. See file.SourceETagSuffix for where it is written down.
//
// When neither side offers an ETag the fallback is size equality. That is
// weaker and is meant to be: it is the difference between re-downloading the
// IERS bulletin every process start and re-downloading it when it actually
// grows, and an endpoint whose content changes without changing size is one
// astrogo would need a checksum for regardless.
func unchanged(ctx context.Context, srcFS, cacheFS FS, name, cacheKey string) bool {
	want, err := fs.Stat(cacheFS, cacheKey)
	if err != nil {
		return false
	}

	got, err := fs.Stat(file.WithContext(ctx, srcFS), name)
	if err != nil {
		return false
	}

	if recorded, current := file.RecordedETag(ctx, cacheFS, cacheKey), file.ETag(got); recorded != "" && current != "" {
		return recorded == current
	}

	return want.Size() > 0 && want.Size() == got.Size()
}

// fetchInto performs the transfer from srcFS/name into cacheFS/cacheKey. It
// owns all policy — consent, timeout, progress, resume, validation — for every
// backend uniformly; a filesystem only moves bytes.
func (c *Client) fetchInto(ctx context.Context, id EndpointID, ep Endpoint, srcFS, cacheFS FS,
	name, cacheKey string, timeout time.Duration, cfg readConfig,
) error {
	// Consent is checked twice: once on the registered estimate before any
	// request, and again below on the size the source actually reports.
	if err := c.CheckDownload(id, name, ep.ApproxSize); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// A Downloadable endpoint's backend must be cancellable, because a
	// multi-gigabyte fetch a caller cannot abandon is a defect rather than an
	// inconvenience. This is the check the storage plan requires, and it fails
	// here — before any byte moves — rather than at ctrl-C.
	bound, err := file.RequireContext(ctx, srcFS)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrDownloadFailed, name, err)
	}

	info, err := fs.Stat(bound, name)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrDownloadFailed, name, err)
	}

	sourceETag := file.ETag(info)

	if err := c.CheckDownload(id, name, info.Size()); err != nil {
		return err
	}

	offset := file.ResumePoint(ctx, cacheFS, cacheKey, sourceETag)

	logging.InfoContext(ctx, "downloading", "cache_key", cacheKey, "endpoint", id, "bytes", info.Size())

	f, err := bound.Open(name)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrDownloadFailed, name, err)
	}

	defer func() { _ = f.Close() }()

	// Resuming means starting the body where the partial stopped. Seek rather
	// than a range parameter: every backend here returns a File, and the HTTP
	// one turns a seek into exactly the ranged GET the old NewRangeReader call
	// issued — so the request on the wire is unchanged and the local backend
	// gets a plain lseek instead of a second open.
	if offset > 0 {
		seeker, ok := f.(io.Seeker)
		if !ok {
			return fmt.Errorf("%w: %s: cannot resume, source is not seekable", ErrDownloadFailed, name)
		}

		if _, err := seeker.Seek(offset, io.SeekStart); err != nil {
			return fmt.Errorf("%w: %s: %w", ErrDownloadFailed, name, err)
		}
	}

	var body io.Reader = f
	if cfg.progress != nil {
		body = &progressReader{r: f, total: info.Size(), read: offset, onProgress: cfg.progress}
	}

	if err := file.StageAndPromote(ctx, cacheFS, cacheKey, body, offset, sourceETag, cfg.validate); err != nil {
		//nolint:wrapcheck // pure delegation to remote/file, internal to this package; its errors are already prefixed
		return err
	}

	// Only a Mutable endpoint keeps its ETag. unchanged() is the sole reader of
	// one, and it runs only for those — so an immutable kernel, which is the
	// multi-gigabyte case and the one a user is most likely to go looking at in
	// their cache directory, gets the object and nothing beside it.
	if !ep.Mutable {
		return nil
	}

	//nolint:wrapcheck // pure delegation to remote/file, internal to this package; its errors are already prefixed
	return file.WriteETag(ctx, cacheFS, cacheKey, sourceETag)
}

// progressReader reports the running byte count after every Read that
// returns data.
type progressReader struct {
	r          io.Reader
	total      int64
	read       int64
	onProgress func(downloaded, total int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.read += int64(n)
		p.onProgress(p.read, p.total)
	}

	//nolint:wrapcheck // must forward io.EOF unwrapped: io.Copy identity-checks it
	return n, err
}
