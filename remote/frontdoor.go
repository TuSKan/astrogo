package remote

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"

	"github.com/TuSKan/astrogo/remote/api"
	"github.com/TuSKan/astrogo/remote/file"
	"github.com/TuSKan/astrogo/time"
)

// This file is the whole of astrogo's I/O surface.
//
// # Why the subpackages are not part of it
//
// remote/file and remote/api are implementation. They move bytes; this package
// decides whether bytes may move at all — which endpoint, whether the process
// is offline, whether a download was consented to, where the result is cached.
// A caller who imported either directly would be reaching past the only place
// those questions are answered.
//
// They stay separate packages because the split is real: a file has a stable
// identity, a size and range semantics, while an API's answer depends on the
// query, and putting both in one file would be one package pretending to be
// two. What changes here is only that the seam is internal.
//
// Nothing outside remote/ may import them, and that is a test rather than a
// convention: TestSubpackagesAreNotImportedDirectly in internal/docsguard
// fails on any such import. The names below are the supported way to reach
// everything they do.

// Bucket is a storage container addressed by URL — a local directory, an HTTP
// prefix, an S3 bucket — with keys inside it.
type Bucket = file.Bucket

// OpenBucket resolves a bucket URL to a [Bucket], reusing one handle per
// distinct URL for the life of the process.
//
// The scheme decides the backend and nothing here does: file://, http:// and
// https:// are always available, and s3:// becomes available by blank-importing
// [github.com/TuSKan/astrogo/remote/file/s3]. An unregistered scheme surfaces
// as that call's own error rather than as a list this package would have to
// keep in step.
func OpenBucket(ctx context.Context, bucketURL string) (*Bucket, error) {
	//nolint:wrapcheck // pure delegation to remote/file, internal to this package; its errors are already prefixed
	return file.Open(ctx, bucketURL)
}

// Save streams r into bucket at key.
//
// Concurrent writers of one file name are serialised, because the driver
// underneath stages every write through a temporary file named from a clock
// that does not advance on Windows. See #241; a caller needs to know only that
// this is safe and a raw bucket write is not.
func Save(ctx context.Context, bucket *Bucket, key string, r io.Reader) error {
	//nolint:wrapcheck // pure delegation to remote/file, internal to this package; its errors are already prefixed
	return file.Save(ctx, bucket, key, r)
}

// IsNotFound reports whether err says the object does not exist.
//
// A cache miss and a broken store are both errors and only one of them is
// normal, so every path that reads before it writes has to tell them apart.
// This is how, and it is the reason no package outside remote needs the
// storage driver's own error package.
func IsNotFound(err error) bool { return file.IsNotFound(err) }

// ReaderAt is a random-access reader over one object that keeps a bounded
// number of chunks resident — 1 MiB for an object of any size, a 3 GB kernel
// included.
//
// It exists because the obvious implementation is quadratically wrong: a range
// request per ReadAt costs one file open under file:// and one HTTP request
// over http:// or S3, measured at 263 ms against 0.33 ms for 2000 SPK-shaped
// reads.
type ReaderAt = file.ReaderAt

// ReaderAtOption configures a [ReaderAt].
type ReaderAtOption = file.ReaderAtOption

// NewReaderAt opens bucket/key for random access. See [ReaderAt].
func NewReaderAt(ctx context.Context, bucket *Bucket, key string, opts ...ReaderAtOption) (*ReaderAt, error) {
	//nolint:wrapcheck // pure delegation to remote/file, internal to this package; its errors are already prefixed
	return file.NewReaderAt(ctx, bucket, key, opts...)
}

// WithChunkSize sets the size of each chunk a [ReaderAt] fetches and caches.
func WithChunkSize(n int64) ReaderAtOption { return file.WithChunkSize(n) }

// WithCachedChunks sets how many chunks a [ReaderAt] keeps resident.
func WithCachedChunks(n int) ReaderAtOption { return file.WithCachedChunks(n) }

// ErrReaderAtClosed reports a read through a [ReaderAt] that was already
// closed.
var ErrReaderAtClosed = file.ErrReaderAtClosed

// The request/response half of this package's surface: SIMBAD, VizieR, Gaia,
// MAST, CelesTrak, FINK, JPL's SBDB and Horizons.
//
// These are methods on [Client] rather than on a client of their own. There was
// a separate APIClient, and keeping it meant every caller held two objects that
// each answered half a question — one deciding whether a service may be
// reached, the other reaching it — and named the endpoint to both. Now that a
// policy is a value a component owns, the connections belong on it: a component
// with its own rules for talking to a service already has somewhere to put
// them.
//
// Every method resolves id through [Client.URL] first, so offline mode,
// [Client.Disable] and a URL override apply per call rather than per client. A
// non-2xx response arrives as an [HTTPError] instead of a body, so a caller
// never parses an error page as data.

// transportFor applies the policy gate to id and returns the transport for it,
// along with the base URL the request goes to.
//
// The gate runs first, so an endpoint this client may not reach never causes a
// transport to be built for it.
func (c *Client) transportFor(id EndpointID) (*api.Client, string, error) {
	base, err := c.URL(id)
	if err != nil {
		return nil, "", err
	}

	// URL succeeded, so the endpoint is registered; a zero Timeout means it
	// registers none and api falls back to its own default.
	ep, _ := c.Lookup(id)

	c.mu.Lock()
	defer c.mu.Unlock()

	if inner, ok := c.transports[id]; ok {
		return inner, base, nil
	}

	inner := api.NewClient(ep.Timeout, c.apiOpts...)
	c.transports[id] = inner

	return inner, base, nil
}

// Get issues a GET against endpoint id and returns the response body, which
// the caller closes.
func Get(ctx context.Context, id EndpointID, path string, query url.Values) (io.ReadCloser, error) {
	return Default().Get(ctx, id, path, query)
}

// Get issues a GET against endpoint id and returns the response body, which
// the caller closes.
func (c *Client) Get(ctx context.Context, id EndpointID, path string, query url.Values) (io.ReadCloser, error) {
	inner, base, err := c.transportFor(id)
	if err != nil {
		return nil, err
	}

	//nolint:wrapcheck // pure delegation to remote/api, internal to this package; its errors are already prefixed
	return inner.Get(ctx, base, path, query)
}

// GetJSON issues a GET against endpoint id and decodes the response into out.
func GetJSON(ctx context.Context, id EndpointID, path string, query url.Values, out any) error {
	return Default().GetJSON(ctx, id, path, query, out)
}

// GetJSON issues a GET against endpoint id and decodes the response into out.
func (c *Client) GetJSON(ctx context.Context, id EndpointID, path string, query url.Values, out any) error {
	inner, base, err := c.transportFor(id)
	if err != nil {
		return err
	}

	//nolint:wrapcheck // pure delegation to remote/api, internal to this package; its errors are already prefixed
	return inner.GetJSON(ctx, base, path, query, out)
}

// PostForm posts form to endpoint id and returns the response body, which the
// caller closes.
func PostForm(ctx context.Context, id EndpointID, path string, form url.Values) (io.ReadCloser, error) {
	return Default().PostForm(ctx, id, path, form)
}

// PostForm posts form to endpoint id and returns the response body, which the
// caller closes.
func (c *Client) PostForm(ctx context.Context, id EndpointID, path string, form url.Values) (io.ReadCloser, error) {
	inner, base, err := c.transportFor(id)
	if err != nil {
		return nil, err
	}

	//nolint:wrapcheck // pure delegation to remote/api, internal to this package; its errors are already prefixed
	return inner.PostForm(ctx, base, path, form)
}

// PostJSON posts payload as JSON to endpoint id and returns the response body,
// which the caller closes.
func PostJSON(ctx context.Context, id EndpointID, path string, payload any) (io.ReadCloser, error) {
	return Default().PostJSON(ctx, id, path, payload)
}

// PostJSON posts payload as JSON to endpoint id and returns the response body,
// which the caller closes.
func (c *Client) PostJSON(ctx context.Context, id EndpointID, path string, payload any) (io.ReadCloser, error) {
	inner, base, err := c.transportFor(id)
	if err != nil {
		return nil, err
	}

	//nolint:wrapcheck // pure delegation to remote/api, internal to this package; its errors are already prefixed
	return inner.PostJSON(ctx, base, path, payload)
}

// Close releases the idle connections of every transport this client opened.
//
// Optional, and usually wrong to call on [Default]: its connections are shared
// by everything in the process that has not built a client of its own, and
// they are reused rather than leaked. A component holding a [Client.Clone] owns
// its connections and may close them when it is done.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var errs []error

	for id, inner := range c.transports {
		if err := inner.Close(); err != nil {
			errs = append(errs, fmt.Errorf("remote: close %s: %w", id, err))
		}
	}

	clear(c.transports)

	return errors.Join(errs...)
}

// APIOption configures the transports a [Client] builds; see
// [Client.SetAPIOptions].
type APIOption = api.Option

// HTTPError is a non-2xx response, carrying its status and body.
type HTTPError = api.HTTPError

// RetryPolicy decides whether a failed attempt is worth repeating.
type RetryPolicy = api.RetryPolicy

// Attempt is one request's outcome, as [RetryPolicy] sees it.
type Attempt = api.Attempt

// DefaultRetryPolicy retries 429, 5xx other than 501, and transport failures.
func DefaultRetryPolicy(a Attempt) bool { return api.DefaultRetryPolicy(a) }

// DefaultAPITimeout is the per-request timeout an endpoint that registers none
// falls back to.
const DefaultAPITimeout = api.DefaultTimeout

// The options an [APIClient] takes. Each forwards to the same option below,
// so the whole configuration surface is reachable without the subpackage.

// WithTimeout overrides the endpoint's registered per-request timeout.
func WithTimeout(d time.Duration) APIOption { return api.WithTimeout(d) }

// WithMinInterval paces requests, leaving at least d between them — for a
// service that asks callers not to hammer it.
func WithMinInterval(d time.Duration) APIOption { return api.WithMinInterval(d) }

// WithAuthToken sets the bearer token sent with every request.
func WithAuthToken(scheme, token string) APIOption { return api.WithAuthToken(scheme, token) }

// WithRetries sets how many times a failed request is repeated.
func WithRetries(n int) APIOption { return api.WithRetries(n) }

// WithUserAgent overrides the User-Agent header.
func WithUserAgent(ua string) APIOption { return api.WithUserAgent(ua) }

// WithRetryPolicy replaces the rule deciding whether an attempt is repeated.
func WithRetryPolicy(p RetryPolicy) APIOption { return api.WithRetryPolicy(p) }
