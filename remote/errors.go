package remote

import (
	"errors"
	"fmt"
)

// Sentinel errors returned by the registry gate and download pipeline.
// Match with errors.Is.
var (
	// ErrOffline is returned by URL (and everything built on it) while
	// global offline mode is active — see SetOffline.
	ErrOffline = errors.New("remote: offline mode enabled")

	// ErrEndpointDisabled is returned when the requested endpoint has been
	// disabled via Disable.
	ErrEndpointDisabled = errors.New("remote: endpoint disabled")

	// ErrUnknownEndpoint is returned for an EndpointID not present in the
	// registry.
	ErrUnknownEndpoint = errors.New("remote: unknown endpoint")

	// ErrDownloadDenied is returned when a file download is blocked by the
	// consent configuration: the endpoint's downloads were never enabled
	// (the default), the file exceeds the configured size limit, or a
	// custom Policy rejected it. The wrapped message states the file, its
	// size, and how to enable the download.
	ErrDownloadDenied = errors.New("remote: download denied")

	// ErrDownloadFailed indicates a download's HTTP exchange or local
	// write failed after the consent checks passed.
	ErrDownloadFailed = errors.New("remote: download failed")

	// ErrRetriable marks a failure that was retried and failed anyway.
	//
	// remote/api's Client wraps it around the final *api.HTTPError whenever the
	// client's retry policy would have retried that status — which, arriving as
	// the final answer, means the attempts ran out or were disabled.
	//
	// The point is that a caller can tell the two kinds of non-2xx apart:
	//
	//	_, err := client.Get(ctx, id, path, nil)
	//	switch {
	//	case errors.Is(err, remote.ErrRetriable):
	//		// The service was busy or broken and we gave up. Worth trying
	//		// later, and worth reporting as an outage rather than a mistake.
	//	case err != nil:
	//		// A 404, a malformed query, a rejected token. Trying again will
	//		// produce exactly the same answer.
	//	}
	//
	// The *api.HTTPError is wrapped rather than replaced, so errors.As still
	// reaches the status and body underneath.
	//
	// Which statuses qualify is [github.com/TuSKan/astrogo/remote/api.RetryPolicy]'s
	// decision; [github.com/TuSKan/astrogo/remote/api.DefaultRetryPolicy] is the
	// answer without one.
	ErrRetriable = errors.New("remote: retriable failure, retries exhausted")

	// ErrNotFileEndpoint is returned by GetFile for an endpoint
	// registered as KindAPI, which has no cache directory or download path.
	ErrNotFileEndpoint = errors.New("remote: not a file endpoint")

	// ErrCacheNameRequired is returned by GetFile when both name and
	// WithCacheName are empty, leaving no cache filename to resolve.
	ErrCacheNameRequired = errors.New("remote: name or WithCacheName required")
)

// HTTPError represents a non-2xx response from an external API endpoint
// that is not retried (or exhausted its retries). The response body is
// captured to aid debugging service-specific error payloads.
//
// Deprecated: use [github.com/TuSKan/astrogo/remote/api.HTTPError], which is
// the one every HTTP path in the module actually returns.
//
// This is a leftover from before remote was split into a policy layer and the
// remote/api transport that moves the bytes. The HTTP exchange went with the
// split and this type did not, so the module has carried two identically named,
// identically shaped errors ever since — one live, one referenced by nothing.
// That is a trap rather than merely dead weight: a caller who reaches for the
// obvious one gets a type nothing ever returns, and errors.As against it fails
// silently on an error whose message is indistinguishable.
type HTTPError struct {
	Body       string
	StatusCode int
}

// Error returns a human-readable string describing the HTTP error.
func (e *HTTPError) Error() string {
	return fmt.Sprintf("remote: http %d - %s", e.StatusCode, e.Body)
}
