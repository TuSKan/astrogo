package file

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/time"
)

// Registered from init — see the note on localFS's registration.
//
//nolint:gochecknoinits // the registry pattern this package is built on
func init() {
	Register("http", openHTTP)
	Register("https", openHTTP)
}

// httpFS serves objects over HTTP, read-only.
//
// # What it is for
//
// Most of astrogo's sources are an HTTP directory: NAIF's kernel listings, the
// IERS bulletin, CelesTrak's element sets. They are read with ranges rather than
// whole — an SPK kernel is gigabytes and a query wants a few hundred kilobytes
// from the middle of it — which is why this implements [File] rather than just
// handing back a body.
//
// It implements neither [CreateFS] nor [RemoveFS]. HTTP PUT and DELETE exist,
// and none of astrogo's sources accept them; a backend that claimed to write
// would be claiming something no endpoint in the registry supports.
type httpFS struct {
	base   *url.URL
	client *http.Client

	// ctx is the context WithContext bound, or nil. See [ContextFS] for why a
	// filesystem carries one at all.
	ctx context.Context //nolint:containedctx // the io/fs contract has nowhere else to put it
}

// openHTTP builds a filesystem rooted at an http:// or https:// URL.
//
// The URL is a directory prefix: a name is resolved beneath it. That matches
// every KindFile endpoint in remote's registry, each of which is documented as
// a directory-style prefix rather than one exact resource.
func openHTTP(u *url.URL) (fs.FS, error) {
	base := *u

	// Strip the query so it cannot leak into every object URL. The two
	// parameters this package understands are handled by the registry, and a
	// server-specific one belongs to the endpoint rather than to each fetch.
	base.RawQuery = ""

	if !strings.HasSuffix(base.Path, "/") {
		base.Path += "/"
	}

	return &httpFS{
		base: &base,
		client: &http.Client{
			// A ceiling, not a policy: a caller with a deadline sets one
			// through ContextFS, and a caller without one should still not
			// hang for ever on a server that accepts a connection and then
			// says nothing.
			Timeout: 30 * time.Minute,
		},
	}, nil
}

// WithContext implements [ContextFS].
func (h *httpFS) WithContext(ctx context.Context) fs.FS {
	clone := *h
	clone.ctx = ctx

	return &clone
}

// Open implements [fs.FS].
//
// The object's size is probed before the file is returned, because [fs.File]
// promises a Stat and [io.Seeker] needs an end to seek relative to. That costs
// one request, and it is the request a caller would have had to make anyway.
func (h *httpFS) Open(name string) (fs.File, error) {
	objURL, err := h.urlFor(name)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}

	header, size, err := h.probe(objURL)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}

	return &httpFile{
		fsys: h,
		url:  objURL,
		info: httpInfo{
			name:    path.Base(name),
			size:    size,
			modTime: lastModified(header),
			header:  header,
		},
	}, nil
}

// Stat implements [fs.StatFS], answering the cache-hit question without
// opening anything.
func (h *httpFS) Stat(name string) (fs.FileInfo, error) {
	objURL, err := h.urlFor(name)
	if err != nil {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: err}
	}

	header, size, err := h.probe(objURL)
	if err != nil {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: err}
	}

	return httpInfo{
		name:    path.Base(name),
		size:    size,
		modTime: lastModified(header),
		header:  header,
	}, nil
}

// context returns the bound context, or a background one.
func (h *httpFS) context() context.Context {
	if h.ctx != nil {
		return h.ctx
	}

	return context.Background()
}

// urlFor resolves an io/fs name against the base.
func (h *httpFS) urlFor(name string) (string, error) {
	if !fs.ValidPath(name) || name == "." {
		return "", fs.ErrInvalid
	}

	ref, err := url.Parse(name)
	if err != nil {
		return "", fs.ErrInvalid
	}

	// A name must not escape the base, and url.ResolveReference would happily
	// let "../.." do it. fs.ValidPath already refuses dot elements, so this is
	// belt and braces against a name that parses into one.
	if ref.IsAbs() || ref.Host != "" || strings.HasPrefix(ref.Path, "/") {
		return "", fs.ErrInvalid
	}

	return h.base.ResolveReference(ref).String(), nil
}

// probe returns an object's headers and size.
//
// # Why this is not just a HEAD
//
// Because a surprising number of servers will not answer one. The fallback is a
// one-byte ranged GET, whose Content-Range carries the total size even when the
// response's own Content-Length describes only the byte returned.
//
// Both halves are needed, and the order matters: HEAD is cheaper and is what a
// well-behaved server wants to be asked, and the ranged GET is what actually
// works against the ones that are not. This is transcribed from astrogo's own
// httpblob driver, where every branch below was put there by a server that
// behaved that way.
func (h *httpFS) probe(objURL string) (http.Header, int64, error) {
	ctx := h.context()

	var headHeader http.Header

	resp, err := h.do(ctx, http.MethodHead, objURL, "")
	if err == nil {
		defer drain(resp)

		if size := contentLength(resp); size >= 0 {
			return resp.Header, size, nil
		}

		// The server answered but would not say how big the object is —
		// chunked transfer encoding sends no Content-Length at all, so
		// "unknown" has to be distinguishable from "empty". Fall through.
		headHeader = resp.Header
	} else if !errors.Is(err, errMethodNotAllowed) {
		return nil, 0, err
	}

	ranged, rangeErr := h.do(ctx, http.MethodGet, objURL, "bytes=0-0")
	if rangeErr != nil {
		if headHeader != nil {
			// HEAD worked and only the size is unknown. Report what there is
			// rather than failing a call that mostly succeeded.
			return headHeader, 0, nil
		}

		return nil, 0, rangeErr
	}

	defer drain(ranged)

	header := ranged.Header
	if headHeader != nil {
		// Prefer HEAD's headers: the ranged response describes one byte.
		header = headHeader
	}

	if total, ok := contentRangeTotal(ranged.Header.Get("Content-Range")); ok {
		return header, total, nil
	}

	if size := contentLength(ranged); size >= 0 {
		return header, size, nil
	}

	return header, 0, nil
}

// errMethodNotAllowed marks a 405, which probe treats as "try the other verb"
// rather than as a failure.
var errMethodNotAllowed = errors.New("remote/file: method not allowed")

// do issues one request and maps its status onto an error.
func (h *httpFS) do(ctx context.Context, method, objURL, rng string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, objURL, nil)
	if err != nil {
		return nil, fmt.Errorf("remote/file: build %s %s: %w", method, objURL, err)
	}

	if rng != "" {
		req.Header.Set("Range", rng)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("remote/file: %s %s: %w", method, objURL, err)
	}

	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		drain(resp)

		return nil, fs.ErrNotExist

	case resp.StatusCode == http.StatusMethodNotAllowed:
		drain(resp)

		return nil, errMethodNotAllowed

	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized:
		drain(resp)

		return nil, fs.ErrPermission

	case resp.StatusCode >= 400:
		drain(resp)

		return nil, fmt.Errorf("remote/file: %s %s: %w: %s",
			method, objURL, ErrHTTPStatus, resp.Status)
	}

	return resp, nil
}

// drain closes a response body, reading what is left so the connection can be
// reused rather than dropped.
func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	_ = resp.Body.Close()
}

// contentLength returns the body length a response declares, or -1 for none.
//
// Chunked transfer encoding sends no Content-Length at all — rclone's WebDAV
// server does exactly this — so "unknown" must be distinguishable from "empty".
func contentLength(resp *http.Response) int64 {
	if resp.ContentLength >= 0 {
		return resp.ContentLength
	}

	if cl := resp.Header.Get("Content-Length"); cl != "" {
		if n, err := strconv.ParseInt(cl, 10, 64); err == nil {
			return n
		}
	}

	return -1
}

// contentRangeTotal extracts the total from "bytes 0-0/1234". It reports false
// for the unknown total "*".
func contentRangeTotal(v string) (int64, bool) {
	i := strings.LastIndex(v, "/")
	if i < 0 {
		return 0, false
	}

	total := strings.TrimSpace(v[i+1:])
	if total == "" || total == "*" {
		return 0, false
	}

	n, err := strconv.ParseInt(total, 10, 64)
	if err != nil {
		return 0, false
	}

	return n, true
}

// lastModified parses the header, returning the zero time when absent or
// unparseable — which fs.FileInfo permits and which is honest about a server
// that did not say.
func lastModified(header http.Header) time.GoTime {
	if header == nil {
		return time.GoTime{}
	}

	t, err := http.ParseTime(header.Get("Last-Modified"))
	if err != nil {
		return time.GoTime{}
	}

	return t
}
