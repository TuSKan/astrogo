package resolve

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v5"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
)

// List of standard catalog/remote interactions errors.
var (
	ErrRateLimited  = errors.New("catalog: rate limited")
	ErrTimeout      = errors.New("catalog: request timeout")
	ErrInvalidInput = errors.New("catalog: invalid input")
	ErrParseFailure = errors.New("catalog: parse failure")
	ErrServiceError = errors.New("catalog: service error")
	ErrRetriable    = errors.New("retriable status code")
)

// HTTPError represents an error returned by an external API endpoint.
type HTTPError struct {
	Body       string
	StatusCode int
}

// Error returns a human-readable string describing the HTTP error.
func (e *HTTPError) Error() string {
	return fmt.Sprintf("catalog: http %d - %s", e.StatusCode, e.Body)
}

// Capability describes what a remote catalog can do.
type Capability string

const (
	// CapObjectResolution indicates that the provider can resolve object names or IDs.
	CapObjectResolution Capability = "ObjectResolution"
	// CapConeSearch indicates that the provider can perform cone searches.
	CapConeSearch Capability = "ConeSearch"
	// CapFullCatalog indicates that the provider can provide full catalog data.
	CapFullCatalog Capability = "FullCatalog"
)

// ObjectRequest represents a request to resolve a specific object name or ID.
type ObjectRequest struct {
	// ID is the unique identifier of the target.
	ID string
	// Query is the name or identifier of the target to resolve.
	Query string
	// Limit is the maximum number of results to return.
	Limit int
}

// ConeRequest represents a spatial query around a specific coordinate.
type ConeRequest struct {
	// ID is the unique identifier of the target.
	ID string
	// Table selects which catalog table a ConeSearcher queries, for
	// providers that support more than one (e.g. catalog/vizier). The
	// empty string means "use the provider's default table" — existing
	// callers that never set this field keep their current behavior
	// unchanged. Providers that don't support table selection ignore this
	// field entirely.
	Table string
	// Center is the coordinate to search around.
	Center coord.ICRS
	// Radius is the search radius.
	Radius angle.Angle
	// Limit is the maximum number of results to return.
	Limit int
}

// ObjectResolver is an advanced remote catalog provider that handles
// asynchronous, cancellable requests natively.
type ObjectResolver interface {
	// Capabilities returns the capabilities of the catalog provider.
	Capabilities() []Capability
	// ResolveObject resolves an object by name or identifier.
	ResolveObject(ctx context.Context, req ObjectRequest) SeqIterator[Target]
}

// ConeSearcher allows radial spatial queries against standard coordinate spaces.
type ConeSearcher interface {
	// Capabilities returns the capabilities of the catalog provider.
	Capabilities() []Capability
	// ConeSearch searches for targets within a given radius of a center coordinate.
	ConeSearch(ctx context.Context, req ConeRequest) SeqIterator[Target]
}

// SeqIterator is an alias for iter.Seq2 for explicit documentation of expected return type.
type SeqIterator[T any] iter.Seq2[T, error]

// SliceSeq converts an in-memory slice to a standard SeqIterator.
func SliceSeq[T any](items []T) SeqIterator[T] {
	return func(yield func(T, error) bool) {
		for _, v := range items {
			if !yield(v, nil) {
				return
			}
		}
	}
}

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

// Client executes HTTP requests with retries, context handling, and rate-limiting support.
type Client struct {
	HTTPClient  *http.Client
	RetryPolicy RetryPolicy
	UserAgent   string
	MaxRetries  uint
}

// NewClient returns a Client with sensible defaults (30s timeout, 3 retries,
// exponential backoff, AstroGo user-agent).
func NewClient() *Client {
	return &Client{
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		MaxRetries:  3,
		RetryPolicy: DefaultRetryPolicy,
		UserAgent:   "AstroGo/1.0",
	}
}

// Do executes the HTTP request with automatic retry and backoff.
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
				return nil, backoff.Permanent(fmt.Errorf("resolve: rewind request body: %w", gbErr))
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
			_ = resp.Body.Close()
			return nil, fmt.Errorf("%w: %d", ErrRetriable, resp.StatusCode)
		}

		return nil, fmt.Errorf("resolve: HTTP do: %w", err)
	}

	// Use backoff/v5 to handle retries and contexts
	result, err := backoff.Retry(req.Context(), operation, backoff.WithMaxTries(c.MaxRetries))
	if err != nil {
		return nil, fmt.Errorf("resolve: retry: %w", err)
	}

	return result, nil
}
