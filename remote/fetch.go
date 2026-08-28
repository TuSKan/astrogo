package remote

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"gocloud.dev/gcerrors"

	"github.com/TuSKan/astrogo/remote/file"
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
// in the cache, returning the cache Bucket and the key within it. The
// caller reads it however it needs — ReadAll, NewReader, file.NewReaderAt.
//
// An immutable endpoint's cache entry is reused on existence alone; a
// Mutable one is revalidated against the source's current ETag first. A
// miss downloads, which requires consent (ErrDownloadDenied otherwise) and
// is serialized against other processes doing the same.
func GetFile(ctx context.Context, id EndpointID, name string, opts ...ReadOption) (bucket *file.Bucket, key string, err error) {
	ep, ok := Lookup(id)
	if !ok {
		return nil, "", fmt.Errorf("%w: %q", ErrUnknownEndpoint, id)
	}

	// URL is the offline/Disable gate. It runs first so a blocked endpoint
	// fails before any cache directory is resolved or lock taken, and so
	// the source below is never opened for a URL the caller may not reach.
	if _, err := URL(id); err != nil {
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

	cacheBucket, prefix, err := CacheDir(ctx, id)
	if err != nil {
		return nil, "", err
	}

	cacheKey := prefix + cacheName

	// An immutable endpoint's cache hit is answered before the source is
	// ever resolved, so a fully-cached kernel stays readable even when its
	// source is unreachable — no network, no credentials, no driver.
	if !ep.Mutable {
		if exists, existsErr := cacheBucket.Exists(ctx, cacheKey); existsErr == nil && exists {
			return cacheBucket, cacheKey, nil
		}
	}

	srcBucket, err := file.Open(ctx, ep.URL)
	if err != nil {
		// A caller who never granted consent gets ErrDownloadDenied rather
		// than this source error even when the source is genuinely
		// unreachable: consent is the actionable blocker from their side,
		// and reporting it consistently matches the documented contract.
		// Routed through CheckDownload so a custom Policy still decides.
		// A caller who did grant consent sees the real error.
		if cerr := CheckDownload(id, name, ep.ApproxSize); cerr != nil {
			return nil, "", cerr
		}

		return nil, "", fmt.Errorf("remote: open source %s: %w", ep.URL, err)
	}

	if fresh, freshErr := freshInCache(ctx, ep, srcBucket, cacheBucket, name, cacheKey); freshErr == nil && fresh {
		return cacheBucket, cacheKey, nil
	}

	// The lock spans the "still missing? then download" decision, not just
	// the transfer: without it two callers both observe the miss and both
	// write. go test runs each package as its own process and several
	// share one JPL kernel, so this is a cross-process race that no
	// in-process mutex can fix.
	release, err := acquireLock(ctx, cacheBucket, cacheKey)
	if err != nil {
		return nil, "", err
	}

	defer release()

	// Whoever held the lock before us may have filled the entry already.
	if fresh, freshErr := freshInCache(ctx, ep, srcBucket, cacheBucket, name, cacheKey); freshErr == nil && fresh {
		return cacheBucket, cacheKey, nil
	}

	timeout := cmp.Or(cfg.timeout, ep.DownloadTimeout, DefaultDownloadTimeout)

	if err := fetchInto(ctx, id, ep, srcBucket, cacheBucket, name, cacheKey, timeout, cfg); err != nil {
		return nil, "", fmt.Errorf("remote: fetch %s: %w", name, err)
	}

	return cacheBucket, cacheKey, nil
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
	ep, ok := Lookup(id)
	if !ok {
		return false, fmt.Errorf("%w: %q", ErrUnknownEndpoint, id)
	}

	if _, err := URL(id); err != nil {
		return false, err
	}

	srcBucket, err := file.Open(ctx, ep.URL)
	if err != nil {
		return false, fmt.Errorf("remote: open source %s: %w", ep.URL, err)
	}

	if _, err := srcBucket.Attributes(ctx, name); err != nil {
		if gcerrors.Code(err) == gcerrors.NotFound {
			return false, nil
		}

		return false, fmt.Errorf("remote: probe %s: %w", name, err)
	}

	return true, nil
}

// freshInCache reports whether cacheKey already holds current content for
// ep+name, so GetFile can skip the transfer entirely.
func freshInCache(ctx context.Context, ep Endpoint, srcBucket, cacheBucket *file.Bucket, name, cacheKey string) (bool, error) {
	exists, err := cacheBucket.Exists(ctx, cacheKey)
	if err != nil {
		return false, fmt.Errorf("remote: check cache %s: %w", cacheKey, err)
	}

	if !exists {
		return false, nil
	}

	if !ep.Mutable {
		return true, nil
	}

	return unchanged(ctx, srcBucket, cacheBucket, name, cacheKey), nil
}

// unchanged compares the source ETag recorded on the cached object at
// fetch time against the source's current one. Any failure — an erroring
// probe, offline mode — reports "changed", so GetFile falls through to its
// normal download path.
//
// The recorded ETag is deliberately not the cached object's own: for a
// local cache, fileblob derives ETag from the file's (ModTime, Size),
// which has nothing to do with the source's, so that comparison would
// never match and every reuse check would degrade into a full re-download.
func unchanged(ctx context.Context, srcBucket, cacheBucket *file.Bucket, name, cacheKey string) bool {
	want, err := cacheBucket.Attributes(ctx, cacheKey)
	if err != nil {
		return false
	}

	got, err := srcBucket.Attributes(ctx, name)
	if err != nil {
		return false
	}

	if recorded := want.Metadata[sourceETagKey]; recorded != "" && got.ETag != "" {
		return recorded == got.ETag
	}

	return want.Size > 0 && want.Size == got.Size
}

// fetchInto performs the transfer from srcBucket/name into
// cacheBucket/cacheKey. It owns all policy — consent, timeout, progress,
// resume, validation — for every backend uniformly; buckets only move
// bytes.
func fetchInto(ctx context.Context, id EndpointID, ep Endpoint, srcBucket, cacheBucket *file.Bucket,
	name, cacheKey string, timeout time.Duration, cfg readConfig,
) error {
	// Consent is checked twice: once on the registered estimate before any
	// request, and again below on the size the source actually reports.
	if err := CheckDownload(id, name, ep.ApproxSize); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	attrs, err := srcBucket.Attributes(ctx, name)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrDownloadFailed, name, err)
	}

	if err := CheckDownload(id, name, attrs.Size); err != nil {
		return err
	}

	offset := resumePoint(ctx, cacheBucket, cacheKey, attrs.ETag)

	log.Printf("remote: downloading %s (endpoint %s, %d bytes)", cacheKey, id, attrs.Size)

	r, err := srcBucket.NewRangeReader(ctx, name, offset, -1, nil)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrDownloadFailed, name, err)
	}

	defer func() { _ = r.Close() }()

	var body io.Reader = r
	if cfg.progress != nil {
		body = &progressReader{r: r, total: offset + r.Size(), read: offset, onProgress: cfg.progress}
	}

	return stageAndPromote(ctx, cacheBucket, cacheKey, body, offset, attrs.ETag, cfg.validate)
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
