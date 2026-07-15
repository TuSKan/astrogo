package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

	timeout := cfg.timeout
	if timeout == 0 {
		timeout = ep.DownloadTimeout
	}

	if timeout == 0 {
		timeout = DefaultDownloadTimeout
	}

	if err := fetchInto(ctx, id, name, cacheFile, timeout, cfg.validate); err != nil {
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
// response streams straight through to Save.
func fetchInto(ctx context.Context, id EndpointID, path string, dest gofs.File, timeout time.Duration, validate func([]byte) error) error {
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

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinURL(base, path), nil)
	if err != nil {
		return fmt.Errorf("remote: new request: %w", err)
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

	if err := CheckDownload(id, name, resp.ContentLength); err != nil {
		return err
	}

	if validate != nil {
		data, err := io.ReadAll(resp.Body)
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

	if err := Save(resp.Body, dest); err != nil {
		return fmt.Errorf("%w: %w", ErrDownloadFailed, err)
	}

	return nil
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
