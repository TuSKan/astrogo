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
	CapObjectResolution Capability = "ObjectResolution"
	CapConeSearch       Capability = "ConeSearch"
	CapFullCatalog      Capability = "FullCatalog"
)

// ObjectRequest represents a request to resolve a specific object name or ID.
type ObjectRequest struct {
	Query string
	Limit int
}

// ConeRequest represents a spatial query around a specific coordinate.
type ConeRequest struct {
	Center coord.ICRS
	Radius angle.Angle
	Limit  int
}

// ObjectResolver is an advanced remote catalog provider that handles
// asynchronous, cancellable requests natively.
type ObjectResolver interface {
	Provider
	Capabilities() []Capability
	ResolveObject(ctx context.Context, req ObjectRequest) SeqIterator[Target]
}

// ConeSearcher allows radial spatial queries against standard coordinate spaces.
type ConeSearcher interface {
	Provider
	Capabilities() []Capability
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
		resp, err := c.HTTPClient.Do(req)

		if !c.RetryPolicy(resp, err) {
			if err != nil {
				return nil, backoff.Permanent(err)
			}
			if resp.StatusCode >= 400 {
				defer resp.Body.Close()
				bdBytes, _ := io.ReadAll(resp.Body)
				return nil, backoff.Permanent(&HTTPError{StatusCode: resp.StatusCode, Body: string(bdBytes)})
			}
			return resp, nil
		}

		if resp != nil && err == nil {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("retriable status code %d", resp.StatusCode)
		}
		return nil, err
	}

	// Use backoff/v5 to handle retries and contexts
	return backoff.Retry(req.Context(), operation, backoff.WithMaxTries(c.MaxRetries))
}
