package remote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	gofs "github.com/ungerik/go-fs"
)

// Signature is a lightweight remote-content fingerprint captured via a HEAD
// request (ETag and/or Content-Length) — the alternative to a wall-clock
// expiration window for sources that actually mutate over time (e.g. the
// IERS EOP bulletin, an upstream catalog CSV). Comparing signatures lets
// GetFile skip a re-download entirely when nothing changed upstream,
// instead of trusting an arbitrary "still fresh enough" age threshold.
type Signature struct {
	ETag          string
	ContentLength int64
}

// readCfg carries per-GetFile options.
type readCfg struct {
	cacheName string
	validate  func([]byte) error
	timeout   time.Duration
	progress  func(downloaded, total int64)
}

// ReadOption customizes a single GetFile call.
type ReadOption func(*readCfg)

// WithCacheName sets the on-disk cache filename when it differs from the
// URL path segment (e.g. IERSFinals2000A: URL is the whole resource,
// path=="", cache file is "finals2000A.data"). Required when name=="".
func WithCacheName(cacheName string) ReadOption {
	return func(c *readCfg) { c.cacheName = cacheName }
}

// WithValidate runs f on freshly downloaded (not cached) bytes before
// they're trusted; on error GetFile returns the error instead of writing
// the cache file or saving a signature, so corrupt content is never
// cached.
func WithValidate(f func([]byte) error) ReadOption {
	return func(c *readCfg) { c.validate = f }
}

// WithDownloadTimeout overrides Endpoint.DownloadTimeout for this one call.
func WithDownloadTimeout(d time.Duration) ReadOption {
	return func(c *readCfg) { c.timeout = d }
}

// WithProgress registers a callback invoked as a download progresses, with
// the bytes transferred so far and the total (0 if unknown, e.g. no
// Content-Length header). Never called for a cache hit.
func WithProgress(f func(downloaded, total int64)) ReadOption {
	return func(c *readCfg) { c.progress = f }
}

// GetFile ensures endpoint id's content at path is present and valid in
// the local cache, then returns the gofs.File itself — the caller opens it
// however it needs (OpenReader for sequential access, OpenReadSeeker for
// random access, ReadAll for whole-content).
//
//   - Endpoint.Mutable == false: the cache is reused if merely present, no
//     HEAD probe (immutable/versioned content — JPL kernels).
//   - Endpoint.Mutable == true: the cache is reused only if a HEAD probe
//     shows nothing changed upstream (IERS, OpenNGC).
//
// A cache miss downloads (consent-gated: ErrDownloadDenied unless
// EnableDownloads was called for id) using Endpoint.DownloadTimeout unless
// overridden by WithDownloadTimeout. With WithValidate, the downloaded
// bytes are buffered and checked before being written to disk (so corrupt
// content is never cached); without it, the transfer streams straight to
// disk without buffering the whole thing in memory (needed for multi-GB
// JPL kernels).
func GetFile(ctx context.Context, id EndpointID, name string, opts ...ReadOption) (gofs.File, error) {
	ep, ok := Lookup(id)
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrUnknownEndpoint, id)
	}

	if !ep.Kind.cacheable() {
		return "", fmt.Errorf("%w: %q", ErrNotFileEndpoint, id)
	}

	tr, err := transportFor(ep.Kind)
	if err != nil {
		return "", err
	}

	var cfg readCfg
	for _, opt := range opts {
		opt(&cfg)
	}

	cacheName := cfg.cacheName
	if cacheName == "" {
		cacheName = name
	}

	if cacheName == "" {
		return "", fmt.Errorf("%w: endpoint %q", ErrCacheNameRequired, id)
	}

	dir, err := CacheDir(id)
	if err != nil {
		return "", err
	}

	cacheFile := dir.Join(cacheName)

	if cacheFile.Exists() && (!ep.Mutable || unchanged(ctx, ep, tr, name, cacheFile)) {
		return cacheFile, nil
	}

	// Hold an exclusive lock across the "still missing? then download"
	// decision, not just the download itself — otherwise two callers can
	// both observe the cache-miss above and both proceed to download.
	// See acquireLock's doc comment for why this must be a cross-process
	// lock, not merely an in-package mutex.
	release, _, lockErr := acquireLock(ctx, cacheFile)
	if lockErr != nil {
		return "", lockErr
	}

	defer release()

	// Re-check: whoever held the lock before us may have already filled
	// this cache entry while we were waiting for it.
	if cacheFile.Exists() && (!ep.Mutable || unchanged(ctx, ep, tr, name, cacheFile)) {
		return cacheFile, nil
	}

	timeout := cfg.timeout
	if timeout == 0 {
		timeout = ep.DownloadTimeout
	}

	if timeout == 0 {
		timeout = DefaultDownloadTimeout
	}

	if err := tr.FetchInto(ctx, id, name, cacheFile, timeout, cfg.validate, cfg.progress); err != nil {
		return "", fmt.Errorf("remote: fetch %s: %w", name, err)
	}

	if ep.Mutable {
		// Best-effort: losing the signature only costs a redundant
		// download next time, so a probe failure here doesn't fail the
		// whole fetch.
		if sig, perr := tr.Probe(ctx, id, name); perr == nil {
			_ = saveSignature(cacheFile, sig)
		}
	}

	return cacheFile, nil
}

// Exists reports whether endpoint id currently serves the file at name.
// It issues a HEAD request (or its transport's equivalent), which
// transfers no body and therefore never triggers the download-consent
// gate — so a caller may use this to discover what is available before
// deciding whether to ask for consent (see
// skybrightness/atlas.NewestVIIRSYear, which probes forward for
// newly-published data years).
//
// A 404 is reported as (false, nil): the endpoint answered, the file just
// is not there. Any other failure — offline mode, a disabled endpoint, a
// network error, a 5xx — returns a non-nil error, so "missing" is never
// confused with "could not tell".
func Exists(ctx context.Context, id EndpointID, name string) (bool, error) {
	if _, err := probeFor(ctx, id, name); err != nil {
		var httpErr *HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

// probeFor resolves id's registered Transport and returns the remote
// Signature for path, so callers holding only an EndpointID (Exists)
// dispatch identically to GetFile's own transport-aware path. A Transport
// implementation must report a not-found path as an *HTTPError with
// StatusCode 404, matching httpTransport's own HEAD-based behavior, so
// this 404-to-(false,nil) mapping in Exists works for every Kind.
func probeFor(ctx context.Context, id EndpointID, path string) (Signature, error) {
	ep, ok := Lookup(id)
	if !ok {
		return Signature{}, fmt.Errorf("%w: %q", ErrUnknownEndpoint, id)
	}

	tr, err := transportFor(ep.Kind)
	if err != nil {
		return Signature{}, err
	}

	sig, err := tr.Probe(ctx, id, path)
	if err != nil {
		return Signature{}, fmt.Errorf("remote: probe %s: %w", path, err)
	}

	return sig, nil
}

// unchanged reports whether the remote content at endpoint ep + path still
// matches the Signature previously recorded for cacheFile — true means the
// caller can skip a full re-download. Comparison prefers ETag when the
// server provides one, falling back to Content-Length otherwise. Any
// failure — no signature recorded yet, the probe erroring, offline mode —
// returns false ("assume changed"), so GetFile always falls through to its
// normal download path. tr is ep's already-resolved Transport (GetFile
// already looked it up), so this never has to look it up again.
func unchanged(ctx context.Context, ep Endpoint, tr Transport, path string, cacheFile gofs.File) bool {
	want := loadSignature(cacheFile)
	if want == (Signature{}) {
		return false
	}

	got, err := tr.Probe(ctx, ep.ID, path)
	if err != nil {
		return false
	}

	if want.ETag != "" && got.ETag != "" {
		return want.ETag == got.ETag
	}

	return want.ContentLength > 0 && want.ContentLength == got.ContentLength
}

// signatureFile returns the sidecar File loadSignature/saveSignature use to
// persist cacheFile's Signature, on the same go-fs filesystem as cacheFile
// itself.
func signatureFile(cacheFile gofs.File) gofs.File {
	return cacheFile + ".signature.json"
}

// loadSignature reads cacheFile's previously recorded Signature, returning
// the zero Signature if none was ever saved (or it's unreadable — never
// treated as fatal, just as "assume changed").
func loadSignature(cacheFile gofs.File) Signature {
	b, err := signatureFile(cacheFile).ReadAll()
	if err != nil {
		return Signature{}
	}

	var sig Signature
	if err := json.Unmarshal(b, &sig); err != nil {
		return Signature{}
	}

	return sig
}

// saveSignature persists sig as cacheFile's Signature sidecar, so a future
// unchanged call has something to compare the remote content against.
func saveSignature(cacheFile gofs.File, sig Signature) error {
	b, err := json.Marshal(sig)
	if err != nil {
		return fmt.Errorf("remote: marshal signature: %w", err)
	}

	if err := signatureFile(cacheFile).WriteAll(b); err != nil {
		return fmt.Errorf("remote: write signature: %w", err)
	}

	return nil
}
