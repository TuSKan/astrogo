package remote

import (
	"maps"
)

// Scope is an immutable snapshot of remote's process-wide configuration —
// every captured endpoint's URL/Enabled/DownloadsOK/MaxDownloadSize, plus
// the global offline flag, download Policy, and data directory — taken by
// Capture and put back by Restore.
//
// This exists because Reset is both too broad and too narrow for scoped
// test use: it wipes every endpoint's consent (including consent a
// package-level TestMain granted for the whole test binary run — a real
// regression this caused once), and it never touches the data directory
// at all (a separate global under its own mutex), so a test that only
// calls Reset can still leak its own SetDataDirPath(t.TempDir()) forward
// into every test that runs afterward. Capture/Restore snapshot exactly
// what was asked for and put exactly that back, nothing more or less.
type Scope struct {
	endpoints  map[EndpointID]Endpoint
	offline    bool
	policy     Policy
	dataDirURL string
}

// Capture snapshots the current configuration. With no arguments it
// captures every registered endpoint; with ids it captures only those,
// leaving every other endpoint's consent untouched by a later Restore —
// the property that matters when a broader scope (e.g. a package
// TestMain) has already granted consent this call must not disturb. The
// global fields (offline, policy, and the data directory) are always
// captured, regardless of ids.
func Capture(ids ...EndpointID) Scope {
	regMu.RLock()
	defer regMu.RUnlock()

	dataMu.RLock()
	defer dataMu.RUnlock()

	var snapshot map[EndpointID]Endpoint

	if len(ids) == 0 {
		snapshot = make(map[EndpointID]Endpoint, len(endpoints))
		for id, ep := range endpoints {
			snapshot[id] = cloneEndpoint(ep)
		}
	} else {
		snapshot = make(map[EndpointID]Endpoint, len(ids))

		for _, id := range ids {
			if ep, ok := endpoints[id]; ok {
				snapshot[id] = cloneEndpoint(ep)
			}
		}
	}

	return Scope{
		endpoints:  snapshot,
		offline:    offline,
		policy:     policy,
		dataDirURL: dataDirURL,
	}
}

// Restore puts back every value Capture recorded: the captured endpoints'
// full configuration (URL, Enabled, DownloadsOK, MaxDownloadSize), offline
// mode, the download Policy, and the data directory. Endpoints outside the
// captured set are never touched.
//
// Restore has a value receiver specifically so it can be used directly as
// a cleanup function: t.Cleanup(remote.Capture().Restore) captures now and
// restores later with no closure needed.
//
// Not safe to use concurrently with anything else mutating remote's
// registry (SetURL, EnableDownloads, SetOffline, ...) — the same caveat
// that already applies to Reset, and why remote-touching tests in this
// module don't run under t.Parallel.
func (s Scope) Restore() {
	regMu.Lock()
	defer regMu.Unlock()

	dataMu.Lock()
	defer dataMu.Unlock()

	maps.Copy(endpoints, s.endpoints)

	offline = s.offline
	policy = s.policy
	dataDirURL = s.dataDirURL
}

// WithScope captures the current configuration, runs fn, and restores the
// captured configuration afterward — even if fn panics.
func WithScope(fn func()) {
	s := Capture()
	defer s.Restore()

	fn()
}
