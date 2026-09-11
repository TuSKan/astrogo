package remote

import (
	"maps"
)

// Scope is an immutable snapshot of a [Client]'s configuration — every
// captured endpoint's URL/Enabled/DownloadsOK/MaxDownloadSize, plus the
// client's offline flag, download Policy, and data directory — taken by
// [Capture] and put back by [Scope.Restore].
//
// This exists because Reset is both too broad and too narrow for scoped
// test use: it wipes every endpoint's consent (including consent a
// package-level TestMain granted for the whole test binary run — a real
// regression this caused once), and it never touches the data directory
// at all, so a test that only calls Reset can still leak its own
// SetDataDir(t.TempDir()) forward into every test that runs afterward.
// Capture/Restore snapshot exactly what was asked for and put exactly that
// back, nothing more or less.
//
// A Scope remembers which client it came from, so Restore puts the values back
// where they were taken from and cannot be pointed at a different one.
type Scope struct {
	client     *Client
	endpoints  map[EndpointID]Endpoint
	policy     Policy
	dataDirURL string
	offline    bool
}

// Capture snapshots [Default]'s current configuration.
func Capture(ids ...EndpointID) Scope { return Default().Capture(ids...) }

// Capture snapshots this client's current configuration. With no arguments it
// captures every registered endpoint; with ids it captures only those,
// leaving every other endpoint's consent untouched by a later Restore —
// the property that matters when a broader scope (e.g. a package
// TestMain) has already granted consent this call must not disturb. The
// client-wide fields (offline, policy, and the data directory) are always
// captured, regardless of ids.
func (c *Client) Capture(ids ...EndpointID) Scope {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var snapshot map[EndpointID]Endpoint

	if len(ids) == 0 {
		snapshot = make(map[EndpointID]Endpoint, len(c.endpoints))
		for id, ep := range c.endpoints {
			snapshot[id] = cloneEndpoint(ep)
		}
	} else {
		snapshot = make(map[EndpointID]Endpoint, len(ids))

		for _, id := range ids {
			if ep, ok := c.endpoints[id]; ok {
				snapshot[id] = cloneEndpoint(ep)
			}
		}
	}

	return Scope{
		client:     c,
		endpoints:  snapshot,
		offline:    c.offline,
		policy:     c.policy,
		dataDirURL: c.dataDirURL,
	}
}

// Restore puts back every value Capture recorded, on the client it was taken
// from: the captured endpoints' full configuration (URL, Enabled, DownloadsOK,
// MaxDownloadSize), offline mode, the download Policy, and the data directory.
// Endpoints outside the captured set are never touched.
//
// Restore has a value receiver specifically so it can be used directly as
// a cleanup function: t.Cleanup(remote.Capture().Restore) captures now and
// restores later with no closure needed.
//
// Not safe to use concurrently with anything else mutating the same client
// (SetURL, EnableDownloads, SetOffline, ...) — the same caveat that already
// applies to Reset, and why remote-touching tests in this module don't run
// under t.Parallel.
func (s Scope) Restore() {
	if s.client == nil {
		return // the zero Scope captured nothing; putting nothing back is correct
	}

	s.client.mu.Lock()
	defer s.client.mu.Unlock()

	maps.Copy(s.client.endpoints, s.endpoints)

	s.client.offline = s.offline
	s.client.policy = s.policy
	s.client.dataDirURL = s.dataDirURL
}

// WithScope captures [Default]'s configuration, runs fn, and restores the
// captured configuration afterward — even if fn panics.
func WithScope(fn func()) { Default().WithScope(fn) }

// WithScope captures this client's configuration, runs fn, and restores the
// captured configuration afterward — even if fn panics.
func (c *Client) WithScope(fn func()) {
	s := c.Capture()
	defer s.Restore()

	fn()
}
