package remote

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v5"
)

// defaultUserAgent identifies astrogo to remote services on every request
// that doesn't set its own User-Agent.
const defaultUserAgent = "AstroGo/1.0"

// RetryPolicy defines whether a request should be retried based on err or response.
type RetryPolicy func(resp *http.Response, err error) bool

// DefaultRetryPolicy retries on transient network errors, 429 (rate-limited),
// and 5xx server errors. Context cancellations are never retried.
func DefaultRetryPolicy(resp *http.Response, err error) bool {
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false
		}

		return true
	}

	if resp != nil {
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			return true
		}
	}

	return false
}

// Client executes HTTP requests with retries, context handling, and
// rate-limiting support. It is the single HTTP execution path for every
// astrogo package that talks to a network service.
type Client struct {
	HTTPClient  *http.Client
	RetryPolicy RetryPolicy
	UserAgent   string
	MaxRetries  uint
}

// ClientOption customizes a Client built by NewClient.
type ClientOption func(*Client)

// WithHTTPClient replaces the underlying *http.Client (custom transport,
// proxy, or TLS configuration).
func WithHTTPClient(h *http.Client) ClientOption {
	return func(c *Client) { c.HTTPClient = h }
}

// WithTimeout sets the underlying client's total request timeout.
func WithTimeout(d time.Duration) ClientOption {
	return func(c *Client) { c.HTTPClient.Timeout = d }
}

// WithMaxRetries sets how many attempts Do makes before giving up.
func WithMaxRetries(n uint) ClientOption {
	return func(c *Client) { c.MaxRetries = n }
}

// WithUserAgent overrides the User-Agent header sent on requests.
func WithUserAgent(ua string) ClientOption {
	return func(c *Client) { c.UserAgent = ua }
}

// NewClient returns a Client with sensible defaults (30s timeout, 3 retries,
// exponential backoff, AstroGo user-agent), customizable via options.
func NewClient(opts ...ClientOption) *Client {
	c := &Client{
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		MaxRetries:  3,
		RetryPolicy: DefaultRetryPolicy,
		UserAgent:   defaultUserAgent,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Do executes the HTTP request with automatic retry and backoff. Non-2xx
// responses that the retry policy classifies as permanent surface as
// *HTTPError (match with errors.As).
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	if c.UserAgent != "" && req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}

	operation := func() (*http.Response, error) {
		// Rewind the request body before every attempt (including the
		// first, a harmless no-op there): net/http drains and closes
		// req.Body on each Do call, so without this, a retry after a
		// transient failure would resend an empty/already-consumed body
		// instead of replaying the original request.
		if req.GetBody != nil {
			body, gbErr := req.GetBody()
			if gbErr != nil {
				return nil, backoff.Permanent(fmt.Errorf("remote: rewind request body: %w", gbErr))
			}

			req.Body = body
		}

		resp, err := c.HTTPClient.Do(req)

		if !c.RetryPolicy(resp, err) {
			if err != nil {
				return nil, backoff.Permanent(err)
			}

			if resp.StatusCode >= 400 {
				bdBytes, _ := io.ReadAll(resp.Body)
				closeErr := resp.Body.Close()

				return nil, backoff.Permanent(errors.Join(
					&HTTPError{StatusCode: resp.StatusCode, Body: string(bdBytes)},
					closeErr,
				))
			}

			return resp, nil
		}

		if resp != nil && err == nil {
			bdBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()

			// Carry the status as a typed *HTTPError so callers that
			// exhaust retries can still map specific statuses (e.g.
			// Horizons' 500/503 sentinels) with errors.As.
			return nil, fmt.Errorf("%w: %w", ErrRetriable,
				&HTTPError{StatusCode: resp.StatusCode, Body: string(bdBytes)})
		}

		return nil, fmt.Errorf("remote: HTTP do: %w", err)
	}

	result, err := backoff.Retry(req.Context(), operation, backoff.WithMaxTries(c.MaxRetries))
	if err != nil {
		return nil, fmt.Errorf("remote: retry: %w", err)
	}

	return result, nil
}

// Get resolves the endpoint's base URL through the registry gate (offline
// mode, enable/disable, overrides), appends path and query, and executes
// the request via Do. It is the standard one-liner for API endpoints.
func (c *Client) Get(ctx context.Context, id EndpointID, path string, query url.Values) (*http.Response, error) {
	base, err := URL(id)
	if err != nil {
		return nil, err
	}

	full := joinURL(base, path)
	if len(query) > 0 {
		full += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return nil, fmt.Errorf("remote: new request: %w", err)
	}

	return c.Do(req)
}

// joinURL appends path to base with exactly one separating slash; an empty
// path returns base unchanged (endpoints like Horizons are complete URLs).
func joinURL(base, path string) string {
	if path == "" {
		return base
	}

	return strings.TrimSuffix(base, "/") + "/" + strings.TrimPrefix(path, "/")
}
