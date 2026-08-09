package remote

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	gofs "github.com/ungerik/go-fs"
)

// httpTransport is remote's built-in stdlib-net/http transport — the one
// registered for KindAPI and KindFile. It owns the Range/If-Range resume
// behavior, ETag capture for Mutable endpoints, streaming-without-
// buffering (unless WithValidate is set), and progress-callback support
// that a Transport implementer for a different Kind (e.g. KindS3's
// remote/s3) is expected to approximate for its own protocol.
type httpTransport struct{}

var _ Transport = httpTransport{}

// FetchInto downloads endpoint id's URL joined with path into dest,
// enforcing astrogo's download-consent rules: the registry gate (offline
// mode, endpoint enabled, URL override), the consent check against the
// endpoint's ApproxSize, then again with the exact Content-Length once
// response headers arrive. With validate non-nil, the full body is
// buffered and validated before being written to dest; otherwise the
// response streams straight through to Save. With progress non-nil, it's
// invoked as the body is read regardless of which of those two paths runs.
func (httpTransport) FetchInto(ctx context.Context, id EndpointID, path string, dest gofs.File, timeout time.Duration, validate func([]byte) error, progress func(downloaded, total int64)) error {
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

// Probe issues a HEAD request against endpoint id's URL joined with path
// and returns its current Signature. A HEAD transfers no body, so it never
// triggers the download-consent check.
func (httpTransport) Probe(ctx context.Context, id EndpointID, path string) (Signature, error) {
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
