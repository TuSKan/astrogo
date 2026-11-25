package remote

import (
	"fmt"
	"slices"
	"sort"
)

// Every exported function in this file is a thin call onto [Default]. The
// state they used to own — the endpoint map, the offline flag, the download
// policy — belongs to a [Client] now, because one policy per process is the
// wrong shape for a binary whose components want different ones (#114). The
// functions stay because a program with a single policy should not have to say
// so.

// Endpoints returns a snapshot of every registered endpoint, sorted by ID, as
// [Default] sees it.
func Endpoints() []Endpoint { return Default().Endpoints() }

// Endpoints returns a snapshot of every registered endpoint, sorted by ID, with
// this client's own decisions applied — its URL overrides, its disables, its
// download grants, and no other client's.
func (c *Client) Endpoints() []Endpoint {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]Endpoint, 0, len(c.endpoints))
	for _, ep := range c.endpoints {
		out = append(out, cloneEndpoint(ep))
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })

	return out
}

// cloneEndpoint copies ep's Files and Mirrors slices so a caller mutating a
// returned Endpoint (or either slice) can never corrupt the registry's own
// copy — every other Endpoint field is a value type and already
// snapshot-safe via Go's plain struct copy.
func cloneEndpoint(ep Endpoint) Endpoint {
	ep.Files = slices.Clone(ep.Files)
	ep.Mirrors = slices.Clone(ep.Mirrors)

	return ep
}

// Lookup returns the endpoint registered under id, as [Default] sees it.
func Lookup(id EndpointID) (Endpoint, bool) { return Default().Lookup(id) }

// Lookup returns the endpoint registered under id, with this client's own
// decisions applied.
func (c *Client) Lookup(id EndpointID) (Endpoint, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	ep, ok := c.endpoints[id]

	return cloneEndpoint(ep), ok
}

// Enable re-enables access to the given endpoints (the default state).
func Enable(ids ...EndpointID) { Default().Enable(ids...) }

// Enable re-enables this client's access to the given endpoints.
func (c *Client) Enable(ids ...EndpointID) { c.setEnabled(true, ids) }

// Disable blocks all access to the given endpoints: any request against
// them fails with ErrEndpointDisabled until re-enabled.
func Disable(ids ...EndpointID) { Default().Disable(ids...) }

// Disable blocks this client's access to the given endpoints. Another client
// holding the same endpoints is unaffected.
func (c *Client) Disable(ids ...EndpointID) { c.setEnabled(false, ids) }

func (c *Client) setEnabled(enabled bool, ids []EndpointID) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, id := range ids {
		if ep, ok := c.endpoints[id]; ok {
			ep.Enabled = enabled
			c.endpoints[id] = ep
		}
	}
}

// SetURL overrides an endpoint's base URL, e.g. to point at a mirror or an
// internal proxy, and clears its [Endpoint.Mirrors]: the URL the caller chose
// is the one asked. Returns ErrUnknownEndpoint for an unregistered id.
func SetURL(id EndpointID, url string) error { return Default().SetURL(id, url) }

// SetURL overrides an endpoint's base URL for this client only, and clears
// its mirrors, as the package-level [SetURL] does. Returns ErrUnknownEndpoint
// for an unregistered id.
func (c *Client) SetURL(id EndpointID, url string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	ep, ok := c.endpoints[id]
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownEndpoint, id)
	}

	ep.URL = url
	ep.Mirrors = nil
	c.endpoints[id] = ep

	return nil
}

// SetOffline toggles global offline mode. While offline, every access to an
// endpoint whose URL reaches the network, API call or download, fails with
// ErrOffline: anything but a file:// path, a mem:// filesystem or an http(s)
// server on the loopback interface (localhost, 127.0.0.0/8, ::1). An endpoint
// pointed with [SetURL] at one of those stays reachable, which is how a test
// serves its fixtures while offline mode keeps everything else on the
// machine.
//
// An object already in the cache is not an access: [GetFile] still serves it,
// which is what lets a pre-seeded, air-gapped deployment run offline.
func SetOffline(off bool) { Default().SetOffline(off) }

// SetOffline toggles offline mode for this client. While offline, every access
// through it to an endpoint whose URL reaches the network fails with
// ErrOffline, a local or loopback endpoint stays reachable, and an object
// already in its cache is still served; see the package-level [SetOffline].
func (c *Client) SetOffline(off bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.offline = off
}

// Offline reports whether global offline mode is active.
func Offline() bool { return Default().Offline() }

// Offline reports whether this client is in offline mode.
func (c *Client) Offline() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.offline
}

// Reset restores the default registry state: every endpoint at its
// built-in URL, online, and no download policy, so every download allowed. It
// does not touch the data directory (see DataDir).
//
// Reset discards what a broader scope — a package TestMain — set for the
// whole binary, offline mode or a policy, so a test using it can break tests
// that run after. Prefer Capture/Restore or WithScope for scoped setup.
func Reset() { Default().Reset() }

// Reset restores this client's registry to the state [NewClient] returns:
// every endpoint at its built-in URL, online, and no download policy. Like the package-level [Reset], it leaves the data directory and the
// API options alone; [Client.Capture] restores the data directory too.
func (c *Client) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.endpoints = defaultEndpoints()
	c.offline = false
	c.policy = nil
}

// URL is the single gate every network call site goes through: it returns
// the endpoint's (possibly overridden) base URL, or an error explaining why
// the endpoint may not be contacted — ErrOffline, ErrEndpointDisabled, or
// ErrUnknownEndpoint.
func URL(id EndpointID) (string, error) { return Default().URL(id) }

// URL is the single gate every call site goes through, for this client: it
// returns the endpoint's (possibly overridden) base URL, or an error explaining
// why this client may not contact it — ErrOffline, ErrEndpointDisabled, or
// ErrUnknownEndpoint.
func (c *Client) URL(id EndpointID) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	ep, ok := c.endpoints[id]
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrUnknownEndpoint, id)
	}

	if !ep.Enabled {
		return "", fmt.Errorf("%w: %q", ErrEndpointDisabled, id)
	}

	if c.offline && reachesNetwork(ep.URL) {
		return "", fmt.Errorf("%w (endpoint %q)", ErrOffline, id)
	}

	return ep.URL, nil
}

// Policy decides whether a file download may proceed. size is the exact
// Content-Length when known, the endpoint's ApproxSize otherwise, or -1
// when neither is available. Returning a non-nil error refuses the download,
// wrapped in ErrDownloadDenied.
type Policy func(ep Endpoint, size int64) error

// SetPolicy installs the download policy, which every file download asks
// before it starts. Without one, and after SetPolicy(nil), every download is
// allowed, of any size. A policy can refuse everything, cap the size, or allow
// only some endpoints:
//
//	remote.SetPolicy(func(ep remote.Endpoint, size int64) error {
//		if size > 500<<20 {
//			return fmt.Errorf("%d bytes is over the 500 MB this program allows", size)
//		}
//
//		return nil
//	})
//
// SetOffline stops all network access, downloads included.
func SetPolicy(p Policy) { Default().SetPolicy(p) }

// SetPolicy installs this client's download policy. Nil allows every download.
func (c *Client) SetPolicy(p Policy) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.policy = p
}

// CheckDownload asks the download policy about a prospective download of the
// given size (semantics as in Policy), returning nil when it may proceed. GetFile calls this itself; it is exported for
// file-producing paths that bypass GetFile — JPLHorizonsSPK generation,
// whose kernel arrives base64-encoded inside a JSON response.
func CheckDownload(id EndpointID, name string, size int64) error {
	return Default().CheckDownload(id, name, size)
}

// CheckDownload asks this client's download policy about a prospective
// download. See the package-level [CheckDownload].
func (c *Client) CheckDownload(id EndpointID, name string, size int64) error {
	c.mu.RLock()

	ep, ok := c.endpoints[id]
	pol := c.policy

	c.mu.RUnlock()

	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownEndpoint, id)
	}

	if pol == nil {
		return nil
	}

	if err := pol(ep, size); err != nil {
		return fmt.Errorf("%w by the policy set with remote.SetPolicy: %s (%s from %s): %w",
			ErrDownloadDenied, name, sizeLabel(size), ep.URL, err)
	}

	return nil
}

// sizeLabel formats a byte count for error messages; negative/unknown sizes
// read as "size unknown".
func sizeLabel(size int64) string {
	switch {
	case size < 0:
		return "size unknown"
	case size >= 1<<30:
		return fmt.Sprintf("~%.1f GB", float64(size)/(1<<30))
	case size >= 1<<20:
		return fmt.Sprintf("~%.0f MB", float64(size)/(1<<20))
	case size >= 1<<10:
		return fmt.Sprintf("~%.0f KB", float64(size)/(1<<10))
	default:
		return fmt.Sprintf("%d bytes", size)
	}
}
