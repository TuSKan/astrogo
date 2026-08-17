package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"time"

	"resty.dev/v3"

	"github.com/TuSKan/astrogo/remote"
)

// DefaultTimeout applies to an endpoint whose registered Timeout is zero.
const DefaultTimeout = 30 * time.Second

// defaultUserAgent identifies astrogo to the services it calls. Several of
// them (Nominatim in particular) require a real one.
const defaultUserAgent = "AstroGo/1.0"

// defaultRetries bounds attempts for a retriable failure.
const defaultRetries = 3

// config carries NewClient's options.
type config struct {
	timeout   time.Duration
	retries   int
	userAgent string
}

// Option customizes a NewClient call.
type Option func(*config)

// WithTimeout overrides the endpoint's registered Timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *config) { c.timeout = d }
}

// WithRetries sets how many times a retriable failure is re-attempted.
// Zero disables retrying.
func WithRetries(n int) Option {
	return func(c *config) { c.retries = n }
}

// WithUserAgent overrides the User-Agent header sent with every request.
func WithUserAgent(ua string) Option {
	return func(c *config) { c.userAgent = ua }
}

// Client talks to one or more registered API endpoints. It is built for a
// specific endpoint — that is where its timeout comes from — but each
// method takes an EndpointID so a provider covering two endpoints of the
// same service (JPL's SBDB identify and query APIs) needs only one client.
//
// Safe for concurrent use.
type Client struct {
	rc *resty.Client
}

// NewClient builds a client configured from endpoint id: its registered
// Timeout, or DefaultTimeout when that is zero, so no caller hand-copies a
// timeout the registry already states. Returns ErrUnknownEndpoint for an
// unregistered id.
func NewClient(id remote.EndpointID, opts ...Option) (*Client, error) {
	ep, ok := remote.Lookup(id)
	if !ok {
		return nil, fmt.Errorf("%w: %q", remote.ErrUnknownEndpoint, id)
	}

	cfg := config{timeout: ep.Timeout, retries: defaultRetries, userAgent: defaultUserAgent}
	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.timeout == 0 {
		cfg.timeout = DefaultTimeout
	}

	// resty's "default conditions" cover only transport, header and URL
	// errors — status-based retrying is opt-in, so the three conditions
	// astrogo wants are added explicitly, using resty's own predicates
	// rather than a hand-rolled copy: a rate limit, any 5xx other than
	// 501 Not Implemented, and a status of zero (no response at all). A
	// 4xx is the caller's own request and is never retried.
	rc := resty.New().
		SetTimeout(cfg.timeout).
		SetRetryCount(cfg.retries).
		SetHeader("User-Agent", cfg.userAgent).
		AddRetryConditions(
			resty.RetryConditionStatusTooManyRequests,
			resty.RetryConditionStatus5XX,
			resty.RetryConditionStatusZero,
		)

	return &Client{rc: rc}, nil
}

// Close releases the client's idle connections. A Client is usually held
// for the life of a provider, so this is optional.
func (c *Client) Close() error {
	if err := c.rc.Close(); err != nil {
		return fmt.Errorf("remote/api: close: %w", err)
	}

	return nil
}

// Get issues a GET against endpoint id and returns the response body. The
// caller closes it. A non-2xx response is returned as an *HTTPError
// instead of a body, so a caller never parses an error page as data.
func (c *Client) Get(ctx context.Context, id remote.EndpointID, path string, query url.Values) (io.ReadCloser, error) {
	full, err := requestURL(id, path)
	if err != nil {
		return nil, err
	}

	resp, err := c.rc.R().
		SetContext(ctx).
		SetQueryParamsFromValues(query).
		SetResponseDoNotParse(true).
		Get(full)

	return body(resp, err)
}

// GetJSON issues a GET and decodes the JSON response into out, closing the
// body. For endpoints that always answer JSON; a provider that must sniff
// the format from raw bytes uses Get instead.
func (c *Client) GetJSON(ctx context.Context, id remote.EndpointID, path string, query url.Values, out any) error {
	r, err := c.Get(ctx, id, path, query)
	if err != nil {
		return err
	}

	defer func() { _ = r.Close() }()

	if err := json.NewDecoder(r).Decode(out); err != nil {
		return fmt.Errorf("remote/api: decode JSON from %q: %w", id, err)
	}

	return nil
}

// PostForm issues a POST with an application/x-www-form-urlencoded body
// and returns the raw response body — the shape TAP-ADQL services and any
// endpoint whose response format must be sniffed need.
func (c *Client) PostForm(ctx context.Context, id remote.EndpointID, path string, form url.Values) (io.ReadCloser, error) {
	full, err := requestURL(id, path)
	if err != nil {
		return nil, err
	}

	resp, err := c.rc.R().
		SetContext(ctx).
		SetFormDataFromValues(form).
		SetResponseDoNotParse(true).
		Post(full)

	return body(resp, err)
}

// PostJSON marshals payload as the JSON request body and returns the raw
// response body.
func (c *Client) PostJSON(ctx context.Context, id remote.EndpointID, path string, payload any) (io.ReadCloser, error) {
	full, err := requestURL(id, path)
	if err != nil {
		return nil, err
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("remote/api: marshal JSON body: %w", err)
	}

	resp, err := c.rc.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(encoded).
		SetResponseDoNotParse(true).
		Post(full)

	return body(resp, err)
}

// requestURL resolves id through the registry gate — the single place
// offline mode, Disable and SetURL are enforced — and appends path.
//
// The join goes through net/url rather than string arithmetic: an endpoint
// whose registered URL already carries a query (a mirror with a token)
// would otherwise have path spliced in after it, producing a URL that
// silently addresses the wrong thing.
func requestURL(id remote.EndpointID, path string) (string, error) {
	base, err := remote.URL(id)
	if err != nil {
		// Wrapped with %w, so a caller's errors.Is against ErrOffline or
		// ErrEndpointDisabled still matches.
		return "", fmt.Errorf("remote/api: %w", err)
	}

	if path == "" {
		return base, nil
	}

	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("remote/api: endpoint %q has an unparseable URL %q: %w", id, base, err)
	}

	return u.JoinPath(path).String(), nil
}

// body turns a resty exchange into the (stream, error) pair every method
// here returns. A non-2xx becomes an *HTTPError carrying the body, since
// these services describe their failures in it.
func body(resp *resty.Response, err error) (io.ReadCloser, error) {
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}

		return nil, fmt.Errorf("remote/api: request failed: %w", err)
	}

	if resp.IsStatusFailure() {
		defer func() { _ = resp.Body.Close() }()

		detail, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))

		return nil, &HTTPError{StatusCode: resp.StatusCode(), Body: string(detail)}
	}

	return resp.Body, nil
}

// maxErrorBody bounds how much of a failed response is captured for the
// error message.
const maxErrorBody = 4096

// HTTPError is a non-2xx response from an API endpoint. The body is
// captured because these services put the actual reason in it — a bad ADQL
// query, an unknown designation, a rate-limit notice.
type HTTPError struct {
	Body       string
	StatusCode int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("remote/api: http %d - %s", e.StatusCode, e.Body)
}
