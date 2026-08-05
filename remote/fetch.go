package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
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

	if ep.Kind != KindFile {
		return "", fmt.Errorf("%w: %q", ErrNotFileEndpoint, id)
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

	if cacheFile.Exists() && (!ep.Mutable || unchanged(ctx, id, name, cacheFile)) {
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
	if cacheFile.Exists() && (!ep.Mutable || unchanged(ctx, id, name, cacheFile)) {
		return cacheFile, nil
	}

	timeout := cfg.timeout
	if timeout == 0 {
		timeout = ep.DownloadTimeout
	}

	if timeout == 0 {
		timeout = DefaultDownloadTimeout
	}

	if err := fetchInto(ctx, id, name, cacheFile, timeout, cfg.validate, cfg.progress); err != nil {
		return "", err
	}

	if ep.Mutable {
		// Best-effort: losing the signature only costs a redundant
		// download next time, so a probe failure here doesn't fail the
		// whole fetch.
		if sig, perr := probe(ctx, id, name); perr == nil {
			_ = saveSignature(cacheFile, sig)
		}
	}

	return cacheFile, nil
}

// fetchInto downloads endpoint id's URL joined with path into dest,
// enforcing astrogo's download-consent rules: the registry gate (offline
// mode, endpoint enabled, URL override), the consent check against the
// endpoint's ApproxSize, then again with the exact Content-Length once
// response headers arrive. With validate non-nil, the full body is
// buffered and validated before being written to dest; otherwise the
// response streams straight through to Save. With progress non-nil, it's
// invoked as the body is read regardless of which of those two paths runs.
func fetchInto(ctx context.Context, id EndpointID, path string, dest gofs.File, timeout time.Duration, validate func([]byte) error, progress func(downloaded, total int64)) error {
	base, err := URL(id)
	if err != nil {
		return err
	}

	name := path
	if name == "" {
		name = dest.Name()
	}

	ep, _ := Lookup(id)
	if err := CheckDownload(id, name, ep.ApproxSize); err != nil {
		return err
	}

	if ep.ApproxSize == SizeVaries {
		log.Printf("remote: downloading %s (endpoint %s, size varies)", dest, id)
	} else {
		log.Printf("remote: downloading %s (endpoint %s, approx %d bytes)", dest, id, ep.ApproxSize)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinURL(base, path), nil)
	if err != nil {
		return fmt.Errorf("remote: new request: %w", err)
	}

	// Resume a previously interrupted transfer. Only the streaming path
	// resumes: the validate path buffers the whole body in memory to check
	// it before anything is trusted, so a partial is useless there.
	// If-Range makes this safe — the server replies 206 only if the ETag
	// still matches, and a plain 200 (changed content, or no range
	// support) transparently restarts the download.
	var resumeOffset int64

	if validate == nil {
		if offset, validator := resumePoint(dest); offset > 0 {
			resumeOffset = offset

			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
			req.Header.Set("If-Range", validator)
		}
	}

	client, err := NewClientFor(id, WithTimeout(timeout))
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrDownloadFailed, name, err)
	}

	defer func() { _ = resp.Body.Close() }()

	// A 206 carries only the remaining bytes, so the consent check (and
	// the progress total) must add back what is already on disk — the
	// gate is about the file's full size, not this leg of it.
	resumed := resp.StatusCode == http.StatusPartialContent
	if !resumed {
		resumeOffset = 0
	}

	total := resp.ContentLength
	if total >= 0 {
		total += resumeOffset
	}

	if err := CheckDownload(id, name, total); err != nil {
		return err
	}

	body := resp.Body

	var bodyReader io.Reader = body

	if progress != nil {
		// read starts at resumeOffset so a resumed transfer reports
		// cumulative progress, not a restart from zero.
		bodyReader = &progressReader{r: body, total: max(total, 0), read: resumeOffset, onProgress: progress}
	}

	if validate != nil {
		data, err := io.ReadAll(bodyReader)
		if err != nil {
			return fmt.Errorf("%w: %s: %w", ErrDownloadFailed, name, err)
		}

		if verr := validate(data); verr != nil {
			return fmt.Errorf("remote: validate %s: %w", name, verr)
		}

		if err := Save(bytes.NewReader(data), dest); err != nil {
			return fmt.Errorf("%w: %w", ErrDownloadFailed, err)
		}

		return nil
	}

	if err := writePartial(bodyReader, dest, resumed, resp.Header.Get("ETag")); err != nil {
		return fmt.Errorf("%w: %w", ErrDownloadFailed, err)
	}

	if err := promotePartial(dest); err != nil {
		return fmt.Errorf("%w: %w", ErrDownloadFailed, err)
	}

	return nil
}

// Exists reports whether endpoint id currently serves the file at name.
// It issues a HEAD request, which transfers no body and therefore never
// triggers the download-consent gate — so a caller may use this to
// discover what is available before deciding whether to ask for consent
// (see skybrightness/atlas.NewestVIIRSYear, which probes forward for
// newly-published data years).
//
// A 404 is reported as (false, nil): the endpoint answered, the file just
// is not there. Any other failure — offline mode, a disabled endpoint, a
// network error, a 5xx — returns a non-nil error, so "missing" is never
// confused with "could not tell".
func Exists(ctx context.Context, id EndpointID, name string) (bool, error) {
	if _, err := probe(ctx, id, name); err != nil {
		var httpErr *HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

// probe issues a HEAD request against endpoint id's URL joined with path
// and returns its current Signature. A HEAD transfers no body, so it never
// triggers the download-consent check.
func probe(ctx context.Context, id EndpointID, path string) (Signature, error) {
	base, err := URL(id)
	if err != nil {
		return Signature{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, joinURL(base, path), nil)
	if err != nil {
		return Signature{}, fmt.Errorf("remote: new HEAD request: %w", err)
	}

	client, err := NewClientFor(id)
	if err != nil {
		return Signature{}, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return Signature{}, fmt.Errorf("remote: HEAD %s: %w", req.URL, err)
	}

	defer func() { _ = resp.Body.Close() }()

	return Signature{ETag: resp.Header.Get("ETag"), ContentLength: resp.ContentLength}, nil
}

// unchanged reports whether the remote content at endpoint id + path still
// matches the Signature previously recorded for cacheFile — true means the
// caller can skip a full re-download. Comparison prefers ETag when the
// server provides one, falling back to Content-Length otherwise. Any
// failure — no signature recorded yet, the probe erroring, offline mode —
// returns false ("assume changed"), so GetFile always falls through to its
// normal download path.
func unchanged(ctx context.Context, id EndpointID, path string, cacheFile gofs.File) bool {
	want := loadSignature(cacheFile)
	if want == (Signature{}) {
		return false
	}

	got, err := probe(ctx, id, path)
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
