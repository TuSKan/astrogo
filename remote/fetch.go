package remote

import (
	"cmp"
	"context"
	"errors"
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
	read      func(fsys FS, key string) error
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
//
// The source version f refused is not downloaded again by this client while
// the source still serves it: GetFile fails with [ErrRejectedSource] after
// asking the source only for its metadata. See [WithRead] for why the record
// lasts as long as the client and no longer.
func WithValidate(f func(io.Reader) error) ReadOption {
	return func(c *readConfig) { c.validate = f }
}

// WithRead hands the copy GetFile found or fetched to read before GetFile
// returns, and takes an error from read to mean the copy is unusable. read is
// the caller's own use of the file, usually its parser; whatever it builds, it
// keeps by closing over a variable. GetFile returns read's error, wrapped
// only when GetFile has more to say.
//
// A copy that read refuses is never left in the cache, where every later call
// would be handed it again. What happens next depends on where it came from:
//
//   - A copy that was already cached may have been damaged where it sits, or
//     left by something other than GetFile. It is removed and fetched once
//     more in the same call, so a corrupt cache heals without the caller
//     noticing.
//   - A copy this call downloaded is what the source serves. It is removed,
//     and its source version (the ETag its source reported) is recorded:
//     until the source serves a different version, GetFile fails with
//     [ErrRejectedSource] after asking only for metadata, instead of
//     downloading the same bytes to fail the same way. One download per
//     source version, not one per call, which for a gigabyte archive is the
//     difference that matters.
//
// The record is this client's and is never written to the cache. A rejection
// is the judgement of the code that made it, and a cached one would outlive
// that code: an astrogo release that learns a source's new format would still
// be refused the file. A new process, or a new client, fetches again.
//
// A source that reports no validator at all cannot be recognized again, so
// such a version is fetched again on every call that rejects it. A read that
// fails because ctx ended says nothing about the copy, which is kept.
func WithRead(read func(fsys FS, key string) error) ReadOption {
	return func(c *readConfig) { c.read = read }
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
//
// In offline mode ([SetOffline]) a cached entry is served without contacting
// the source, Mutable or not, and only a miss fails, with ErrOffline.
func GetFile(ctx context.Context, id EndpointID, name string, opts ...ReadOption) (fsys FS, key string, err error) {
	return Default().GetFile(ctx, id, name, opts...)
}

// GetFile ensures endpoint id's object named name is present and current in
// this client's cache, returning that cache [FS] and the key within it. See
// the package-level [GetFile]; the consent, offline and URL decisions it makes
// are this client's, and the cache it fills is this client's.
func (c *Client) GetFile(ctx context.Context, id EndpointID, name string, opts ...ReadOption) (fsys FS, key string, err error) {
	var cfg readConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	fsys, key, got, err := c.getFile(ctx, id, name, cfg)
	if err != nil || cfg.read == nil {
		return fsys, key, err
	}

	return c.readCopy(ctx, id, name, cfg, fsys, key, got)
}

// fetched describes the download one getFile call made: none, or one of the
// source version named by sourceETag.
type fetched struct {
	ok         bool
	sourceETag string
}

// readCopy hands the copy getFile returned to cfg.read, and deals with one that
// read refuses as [WithRead] documents: a copy that was already cached is
// removed and fetched once more, and a copy just downloaded is removed and its
// source version rejected.
func (c *Client) readCopy(ctx context.Context, id EndpointID, name string, cfg readConfig,
	fsys FS, key string, got fetched,
) (FS, string, error) {
	readErr := cfg.read(fsys, key)

	switch {
	case readErr == nil:
		return fsys, key, nil
	case ctx.Err() != nil:
		return nil, "", readErr
	}

	if err := c.discard(ctx, fsys, key, got, readErr); err != nil {
		return nil, "", errors.Join(readErr, err)
	}

	if got.ok {
		return nil, "", readErr
	}

	fsys, key, got, err := c.getFile(ctx, id, name, cfg)
	if err != nil {
		return nil, "", fmt.Errorf("%w; fetching a fresh copy: %w", readErr, err)
	}

	readErr = cfg.read(fsys, key)

	switch {
	case readErr == nil:
		return fsys, key, nil
	case ctx.Err() != nil:
		return nil, "", readErr
	}

	if err := c.discard(ctx, fsys, key, got, readErr); err != nil {
		return nil, "", errors.Join(readErr, err)
	}

	return nil, "", readErr
}

// discard removes a copy cfg.read refused, with the source ETag recorded beside
// it, first rejecting its source version when this call is the one that
// downloaded it.
func (c *Client) discard(ctx context.Context, fsys FS, key string, got fetched, readErr error) error {
	if got.ok {
		c.reject(key, got.sourceETag, readErr)
	}

	if err := file.Remove(ctx, fsys, key); err != nil {
		//nolint:wrapcheck // pure delegation to remote/file, internal to this package; its errors are already prefixed
		return err
	}

	// Only a Mutable endpoint's copy has one, and a leftover would be inert,
	// but nothing of a refused copy is left behind.
	if err := file.Remove(ctx, fsys, key+file.SourceETagSuffix); err != nil && !errors.Is(err, fs.ErrNotExist) {
		//nolint:wrapcheck // pure delegation to remote/file, internal to this package; its errors are already prefixed
		return err
	}

	return nil
}

// getFile is GetFile without cfg.read: it finds the object in the cache or
// fetches it, and reports which.
func (c *Client) getFile(ctx context.Context, id EndpointID, name string, cfg readConfig) (fsys FS, key string, got fetched, err error) {
	ep, ok := c.Lookup(id)
	if !ok {
		return nil, "", fetched{}, fmt.Errorf("%w: %q", ErrUnknownEndpoint, id)
	}

	// URL is the offline/Disable gate. A disabled endpoint fails here,
	// before any cache directory is resolved or lock taken, and the source
	// below is never opened for a URL the caller may not reach.
	//
	// Offline mode alone waits for the cache to be looked at. It forbids
	// contacting the source, not reading what is already here: the
	// air-gapped deployment the README documents pre-seeds the cache and
	// then goes offline, and until #633 every pre-seeded kernel was refused
	// with ErrOffline.
	_, gateErr := c.URL(id)

	offline := errors.Is(gateErr, ErrOffline) && ep.Enabled
	if gateErr != nil && !offline {
		return nil, "", fetched{}, gateErr
	}

	if !ep.Kind.cacheable() {
		return nil, "", fetched{}, fmt.Errorf("%w: %q", ErrNotFileEndpoint, id)
	}

	cacheName := cfg.cacheName
	if cacheName == "" {
		cacheName = name
	}

	if cacheName == "" {
		return nil, "", fetched{}, fmt.Errorf("%w: endpoint %q", ErrCacheNameRequired, id)
	}

	cacheFS, prefix, err := c.CacheDir(ctx, id)
	if err != nil {
		return nil, "", fetched{}, err
	}

	cacheKey := prefix + cacheName

	// An immutable endpoint's cache hit is answered before the source is
	// ever resolved, so a fully-cached kernel stays readable even when its
	// source is unreachable — no network, no credentials, no driver.
	// Offline, a Mutable endpoint's cached object is served too: it cannot
	// be revalidated without the source, and offline mode is the caller
	// saying there is none to ask.
	if !ep.Mutable || offline {
		if exists, existsErr := file.Exists(ctx, cacheFS, cacheKey); existsErr == nil && exists {
			return cacheFS, cacheKey, fetched{}, nil
		}
	}

	if offline {
		return nil, "", fetched{}, gateErr
	}

	// Consent first, before any request, routed through CheckDownload so a
	// custom Policy still decides.
	//
	// A caller who may not download can only be given what is already here.
	// Asking a mutable source whether its cached copy is current gives such
	// a caller nothing it could act on: a request it never agreed to and,
	// against a source that does not answer, the whole of the endpoint's
	// timeout. The lazy EOP load sat out 17 s of it with every uncovered
	// lookup in the process queued behind it, to be handed the bulletin its
	// own cached read had just found (#964). So it is served what is cached,
	// as offline mode serves it, and refused only when nothing is.
	//
	// Refused before the source is even opened, so the error is consent's
	// whether or not the source is reachable, the actionable blocker from
	// the caller's side. And before the lock, which another process can hold
	// for as long as its download takes, or a crashed one until the lock goes
	// stale after half an hour, all of it to be refused at the end (#424).
	// fetchInto checks again, on the size the source reports.
	if err := c.CheckDownload(id, name, ep.ApproxSize); err != nil {
		if exists, existsErr := file.Exists(ctx, cacheFS, cacheKey); existsErr == nil && exists {
			return cacheFS, cacheKey, fetched{}, nil
		}

		return nil, "", fetched{}, err
	}

	srcFS, err := OpenFS(ctx, ep.URL)
	if err != nil {
		return nil, "", fetched{}, fmt.Errorf("remote: open source %s: %w", ep.URL, err)
	}

	if fresh, freshErr := freshInCache(ctx, ep, srcFS, cacheFS, name, cacheKey); freshErr == nil && fresh {
		return cacheFS, cacheKey, fetched{}, nil
	}

	// The lock spans the "still missing? then download" decision, not just
	// the transfer: without it two callers both observe the miss and both
	// write. go test runs each package as its own process and several
	// share one JPL kernel, so this is a cross-process race that no
	// in-process mutex can fix.
	// Every holder of this lock downloads under the same timeout, so one older
	// than twice it is not downloading: it crashed, and its lock is stolen
	// rather than waited out — for the IERS bulletin's 30 s, after a minute
	// instead of half an hour, the lazy EOP load's mutex held the while (#445).
	timeout := cmp.Or(cfg.timeout, ep.DownloadTimeout, DefaultDownloadTimeout)

	release, err := file.AcquireLock(ctx, cacheFS, cacheKey, 2*timeout)
	if err != nil {
		//nolint:wrapcheck // pure delegation to remote/file, internal to this package; its errors are already prefixed
		return nil, "", fetched{}, err
	}

	defer release()

	// Whoever held the lock before us may have filled the entry already.
	if fresh, freshErr := freshInCache(ctx, ep, srcFS, cacheFS, name, cacheKey); freshErr == nil && fresh {
		return cacheFS, cacheKey, fetched{}, nil
	}

	sourceETag, err := c.fetchInto(ctx, id, ep, srcFS, cacheFS, name, cacheKey, timeout, cfg)
	if err != nil {
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
			return cacheFS, cacheKey, fetched{}, nil
		}

		return nil, "", fetched{}, fmt.Errorf("remote: fetch %s: %w", name, err)
	}

	return cacheFS, cacheKey, fetched{ok: true, sourceETag: sourceETag}, nil
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
// backend uniformly; a filesystem only moves bytes. It returns the source
// version it fetched, the ETag the source reported for it.
func (c *Client) fetchInto(ctx context.Context, id EndpointID, ep Endpoint, srcFS, cacheFS FS,
	name, cacheKey string, timeout time.Duration, cfg readConfig,
) (sourceETag string, err error) {
	// Consent is checked twice: once on the registered estimate before any
	// request, and again below on the size the source actually reports.
	if err := c.CheckDownload(id, name, ep.ApproxSize); err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// A Downloadable endpoint's backend must be cancellable, because a
	// multi-gigabyte fetch a caller cannot abandon is a defect rather than an
	// inconvenience. This is the check the storage plan requires, and it fails
	// here — before any byte moves — rather than at ctrl-C.
	bound, err := file.RequireContext(ctx, srcFS)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %w", ErrDownloadFailed, name, err)
	}

	info, err := fs.Stat(bound, name)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %w", ErrDownloadFailed, name, err)
	}

	sourceETag = file.ETag(info)

	// The same bytes would fail the same way, so the body is not fetched.
	if r, rejected := c.rejection(cacheKey, sourceETag); rejected {
		return "", fmt.Errorf("%w: %s, at source version %s, failed: %s", ErrRejectedSource, name, sourceETag, r.cause)
	}

	if err := c.CheckDownload(id, name, info.Size()); err != nil {
		return "", err
	}

	offset, err := file.ResumePoint(ctx, cacheFS, cacheKey, sourceETag)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %w", ErrDownloadFailed, name, err)
	}

	logging.InfoContext(ctx, "downloading", "cache_key", cacheKey, "endpoint", id, "bytes", info.Size())

	f, err := bound.Open(name)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %w", ErrDownloadFailed, name, err)
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
			return "", fmt.Errorf("%w: %s: cannot resume, source is not seekable", ErrDownloadFailed, name)
		}

		if _, err := seeker.Seek(offset, io.SeekStart); err != nil {
			return "", fmt.Errorf("%w: %s: %w", ErrDownloadFailed, name, err)
		}
	}

	var body io.Reader = f
	if cfg.progress != nil {
		body = &progressReader{r: f, total: info.Size(), read: offset, onProgress: cfg.progress}
	}

	// Only the validator's own refusal rejects the version. A transfer or a
	// write that failed says nothing about what the source serves.
	validate, refused := cfg.validate, error(nil)
	if validate != nil {
		validate = func(r io.Reader) error {
			refused = cfg.validate(r)

			return refused
		}
	}

	if err := file.StageAndPromote(ctx, cacheFS, cacheKey, body, offset, sourceETag, validate); err != nil {
		if refused != nil && ctx.Err() == nil {
			c.reject(cacheKey, sourceETag, refused)
		}

		//nolint:wrapcheck // pure delegation to remote/file, internal to this package; its errors are already prefixed
		return "", err
	}

	// A version GetFile could fetch is not one it rejected: rejection stops
	// the fetch above. Whatever was rejected under this key is gone upstream.
	c.forgetRejection(cacheKey)

	// Only a Mutable endpoint keeps its ETag. unchanged() is the sole reader of
	// one, and it runs only for those — so an immutable kernel, which is the
	// multi-gigabyte case and the one a user is most likely to go looking at in
	// their cache directory, gets the object and nothing beside it.
	if !ep.Mutable {
		return sourceETag, nil
	}

	if err := file.WriteETag(ctx, cacheFS, cacheKey, sourceETag); err != nil {
		//nolint:wrapcheck // pure delegation to remote/file, internal to this package; its errors are already prefixed
		return "", err
	}

	return sourceETag, nil
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
