package remote

import (
	"errors"

	"github.com/TuSKan/astrogo/remote/api"
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
	//
	// The value is remote/api's own, so errors.Is matches whichever name a
	// caller reached for: the wrapping happens down there and the documented
	// name is up here.
	ErrRetriable = api.ErrRetriable

	// ErrNotFileEndpoint is returned by GetFile for an endpoint
	// registered as KindAPI, which has no cache directory or download path.
	ErrNotFileEndpoint = errors.New("remote: not a file endpoint")

	// ErrCacheNameRequired is returned by GetFile when both name and
	// WithCacheName are empty, leaving no cache filename to resolve.
	ErrCacheNameRequired = errors.New("remote: name or WithCacheName required")
)
