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

	// ErrRetriable wraps a retriable HTTP status inside the retry loop.
	// It normally never escapes Client.Do; it is exported so custom
	// RetryPolicy implementations can produce/detect it.
	ErrRetriable = errors.New("remote: retriable status code")
)

// HTTPError represents a non-2xx response from an external API endpoint
// that is not retried (or exhausted its retries). The response body is
// captured to aid debugging service-specific error payloads.
type HTTPError struct {
	Body       string
	StatusCode int
}

// Error returns a human-readable string describing the HTTP error.
func (e *HTTPError) Error() string {
	return fmt.Sprintf("remote: http %d - %s", e.StatusCode, e.Body)
}
