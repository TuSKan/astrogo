package remote

import (
	"context"
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

// APIClient issues requests against a request/response service — SIMBAD,
// VizieR, Gaia, MAST, CelesTrak, FINK, JPL's SBDB and Horizons.
//
// Every method resolves id through [URL] and then makes the request, so
// offline mode, [Disable] and a URL override apply per call rather than per
// client: a client built while an endpoint was reachable starts failing the
// moment it is not, and starts working again when [SetURL] points it somewhere
// that is. A non-2xx response arrives as an [HTTPError] instead of a body, so
// a caller never parses an error page as data.
//
// # Why four two-line methods and not an alias
//
// The client underneath takes a base URL per call and knows nothing about
// endpoints — that is what broke the import cycle and let this package become
// the only door. Aliasing it would hand callers the base-URL parameter and the
// job of resolving it, which is the one job this package exists to do; every
// call site would then be free to skip the gate, and [EndpointID] would stop
// meaning anything.
//
// So the resolution below is the whole point of the wrapper, not overhead
// around it.
type APIClient struct {
	c *api.Client

	// owner is the policy each request resolves against. Held rather than
	// looked up so a client built from a scoped [Client] keeps answering to
	// that one, not to whatever [Default] has become since.
	owner *Client
}

// NewAPIClient builds a client for endpoint id, taking its registered timeout.
//
// The lookup here is for that timeout and to reject an id that is not in the
// registry at all. It is not the policy gate — that runs per request, so an
// endpoint disabled or an offline mode set after construction takes effect on
// the next call rather than the next client. A client may therefore be built
// successfully while offline and fail on use, which is the intended order:
// construction is not the moment a caller is asking to reach the network.
func NewAPIClient(id EndpointID, opts ...APIOption) (*APIClient, error) {
	return Default().NewAPIClient(id, opts...)
}

// NewAPIClient builds a client for endpoint id whose every request resolves
// against this client's policy. See the package-level [NewAPIClient].
func (c *Client) NewAPIClient(id EndpointID, opts ...APIOption) (*APIClient, error) {
	ep, ok := c.Lookup(id)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownEndpoint, id)
	}

	return &APIClient{c: api.NewClient(ep.Timeout, opts...), owner: c}, nil
}

// Get issues a GET against endpoint id and returns the response body, which
// the caller closes.
func (c *APIClient) Get(ctx context.Context, id EndpointID, path string, query url.Values) (io.ReadCloser, error) {
	base, err := c.owner.URL(id)
	if err != nil {
		return nil, err
	}

	//nolint:wrapcheck // pure delegation to remote/api, internal to this package; its errors are already prefixed
	return c.c.Get(ctx, base, path, query)
}

// GetJSON issues a GET against endpoint id and decodes the response into out.
func (c *APIClient) GetJSON(ctx context.Context, id EndpointID, path string, query url.Values, out any) error {
	base, err := c.owner.URL(id)
	if err != nil {
		return err
	}

	//nolint:wrapcheck // pure delegation to remote/api, internal to this package; its errors are already prefixed
	return c.c.GetJSON(ctx, base, path, query, out)
}

// PostForm posts form to endpoint id and returns the response body, which the
// caller closes.
func (c *APIClient) PostForm(ctx context.Context, id EndpointID, path string, form url.Values) (io.ReadCloser, error) {
	base, err := c.owner.URL(id)
	if err != nil {
		return nil, err
	}

	//nolint:wrapcheck // pure delegation to remote/api, internal to this package; its errors are already prefixed
	return c.c.PostForm(ctx, base, path, form)
}

// PostJSON posts payload as JSON to endpoint id and returns the response body,
// which the caller closes.
func (c *APIClient) PostJSON(ctx context.Context, id EndpointID, path string, payload any) (io.ReadCloser, error) {
	base, err := c.owner.URL(id)
	if err != nil {
		return nil, err
	}

	//nolint:wrapcheck // pure delegation to remote/api, internal to this package; its errors are already prefixed
	return c.c.PostJSON(ctx, base, path, payload)
}

// Close releases the client's idle connections. A client is usually held for
// the life of a provider, so this is optional.
//
//nolint:wrapcheck // pure delegation to remote/api, internal to this package; its errors are already prefixed
func (c *APIClient) Close() error { return c.c.Close() }

// APIOption configures an [APIClient].
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
