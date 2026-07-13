package remote

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	gofs "github.com/ungerik/go-fs"
)

// downloadTimeout bounds a whole download (connect + transfer). Kernel
// files can be large (hundreds of MB), so this is generous compared to a
// typical API-call timeout — its purpose is only to prevent an indefinite
// hang on a stalled connection, not to cap legitimately slow transfers.
const downloadTimeout = 10 * time.Minute

// downloadCfg carries per-download options.
type downloadCfg struct {
	timeout  time.Duration
	progress func(written, total int64)
}

// DownloadOption customizes a single Download/DownloadURL call.
type DownloadOption func(*downloadCfg)

// WithDownloadTimeout bounds the whole transfer (default 10 minutes).
func WithDownloadTimeout(d time.Duration) DownloadOption {
	return func(c *downloadCfg) { c.timeout = d }
}

// WithProgress installs a progress callback invoked as bytes arrive. total
// is the Content-Length, or -1 when the server didn't declare one.
func WithProgress(f func(written, total int64)) DownloadOption {
	return func(c *downloadCfg) { c.progress = f }
}

// Download fetches endpoint id's URL joined with path into dest,
// enforcing astrogo's download-consent rules:
//
//  1. the registry gate (offline mode, endpoint enabled, URL override),
//  2. the consent check against the endpoint's ApproxSize — downloads are
//     DENIED unless EnableDownloads was called for this endpoint,
//  3. after response headers arrive, the consent check again with the
//     exact Content-Length (so a size limit holds even when ApproxSize
//     was unknown),
//  4. streaming to a temp file and an atomic rename into place.
//
// dest may live on any go-fs filesystem; for the local filesystem the
// rename is atomic.
func Download(ctx context.Context, id EndpointID, path string, dest gofs.File, opts ...DownloadOption) error {
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

	return download(ctx, id, name, joinURL(base, path), dest, opts)
}

// DownloadURL fetches rawURL into dest without a registry entry. It is the
// low-level form used by explicit developer tooling (the go:generate
// download helper) — calling it IS the consent, so no endpoint gate or
// size policy applies beyond global offline mode.
func DownloadURL(ctx context.Context, rawURL string, dest gofs.File, opts ...DownloadOption) error {
	if Offline() {
		return fmt.Errorf("%w (url %s)", ErrOffline, rawURL)
	}

	return download(ctx, "", dest.Name(), rawURL, dest, opts)
}

// download runs the shared transfer pipeline. A non-empty id re-checks the
// consent policy once the exact Content-Length is known.
func download(ctx context.Context, id EndpointID, name, rawURL string, dest gofs.File, opts []DownloadOption) (err error) {
	cfg := downloadCfg{timeout: downloadTimeout}
	for _, opt := range opts {
		opt(&cfg)
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("remote: new request: %w", err)
	}

	req.Header.Set("User-Agent", defaultUserAgent)

	// Reuse Client's retry/backoff for the connection + status phase; the
	// body streams below outside the retry loop (a mid-body failure of a
	// multi-hundred-MB transfer is not silently re-run).
	client := NewClient(WithTimeout(cfg.timeout))

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrDownloadFailed, name, err)
	}

	defer func() {
		cerr := resp.Body.Close()
		if cerr != nil {
			err = errors.Join(err, cerr)
		}
	}()

	if id != "" {
		if cerr := CheckDownload(id, name, resp.ContentLength); cerr != nil {
			return cerr
		}
	}

	if err := dest.Dir().MakeAllDirs(); err != nil {
		return fmt.Errorf("remote: mkdir %s: %w", dest.Dir(), err)
	}

	body := io.Reader(resp.Body)
	if cfg.progress != nil {
		body = &progressReader{r: resp.Body, total: resp.ContentLength, fn: cfg.progress}
	}

	if local := dest.LocalPath(); local != "" {
		return downloadToLocal(body, local)
	}

	return downloadToFS(body, dest)
}

// downloadToLocal streams into a temp file next to path and atomically
// renames it into place — never leaving a partial file at path.
func downloadToLocal(body io.Reader, path string) (err error) {
	tmpFile, err := os.CreateTemp(filepath.Dir(path), "download-*.tmp")
	if err != nil {
		return fmt.Errorf("remote: create temp: %w", err)
	}

	tmpName := tmpFile.Name()

	// Ensure we don't leak the tmp file if something panics or fails early.
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpName)
	}()

	if _, err = io.Copy(tmpFile, body); err != nil {
		return fmt.Errorf("%w: copy: %w", ErrDownloadFailed, err)
	}

	// Close explicitly before rename (critical for Windows).
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("remote: close temp: %w", err)
	}

	// Atomically move the fully downloaded file into place.
	if err := os.Rename(tmpName, path); err != nil {
		// On Windows, if multiple processes run concurrently, another one
		// might have already downloaded and opened (locked) the file. If
		// the file exists and has a positive size, the rename loss is
		// harmless.
		if stat, statErr := os.Stat(path); statErr == nil && stat.Size() > 0 {
			return nil
		}

		return fmt.Errorf("remote: finalize download atomic rename: %w", err)
	}

	return nil
}

// downloadToFS streams into a non-local go-fs destination (e.g. a future
// blob/bucket filesystem), where atomic rename is generally unavailable —
// the write goes directly to dest.
func downloadToFS(body io.Reader, dest gofs.File) (err error) {
	w, err := dest.OpenWriter()
	if err != nil {
		return fmt.Errorf("remote: open writer %s: %w", dest, err)
	}

	defer func() {
		cerr := w.Close()
		if cerr != nil {
			err = errors.Join(err, cerr)
		}
	}()

	if _, err := io.Copy(w, body); err != nil {
		return fmt.Errorf("%w: copy: %w", ErrDownloadFailed, err)
	}

	return nil
}

// progressReader invokes fn as bytes flow through it.
type progressReader struct {
	r       io.Reader
	fn      func(written, total int64)
	written int64
	total   int64
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.written += int64(n)
		p.fn(p.written, p.total)
	}

	return n, err //nolint:wrapcheck // transparent io.Reader pass-through; wrapping would break io.EOF identity for some callers
}
