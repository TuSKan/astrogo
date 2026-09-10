package remote

// The package-level policy functions, each operating on [Default].
//
// This file is deliberately nothing but delegation. Every one of these
// predates [Client] and is the whole API for a program with a single policy,
// which is most of them; keeping the names, signatures and documented
// behaviour unchanged is what makes #114 a shape change rather than a break.
//
// The pattern is [net/http]'s: [net/http.Get] is [net/http.DefaultClient.Get],
// and nobody constructing an [net/http.Client] is being told the package
// function was wrong.
//
// Where the doc comment lives is on the method, not here — one description of
// each behaviour, attached to the implementation, rather than two that can
// disagree.

// Endpoints returns a snapshot of every endpoint registered on [Default].
// See [Client.Endpoints].
func Endpoints() []Endpoint { return Default().Endpoints() }

// Lookup returns the endpoint [Default] has registered under id.
// See [Client.Lookup].
func Lookup(id EndpointID) (Endpoint, bool) { return Default().Lookup(id) }

// Enable re-enables access to the given endpoints on [Default].
// See [Client.Enable].
func Enable(ids ...EndpointID) { Default().Enable(ids...) }

// Disable blocks all access to the given endpoints on [Default].
// See [Client.Disable].
func Disable(ids ...EndpointID) { Default().Disable(ids...) }

// SetURL overrides an endpoint's base URL on [Default].
// See [Client.SetURL].
func SetURL(id EndpointID, url string) error { return Default().SetURL(id, url) }

// SetOffline toggles offline mode on [Default].
// See [Client.SetOffline].
func SetOffline(off bool) { Default().SetOffline(off) }

// Offline reports whether [Default] is in offline mode.
// See [Client.Offline].
func Offline() bool { return Default().Offline() }

// Reset restores [Default] to astrogo's built-in policy.
// See [Client.Reset].
func Reset() { Default().Reset() }

// URL returns an endpoint's usable base URL on [Default], or the error
// explaining why it may not be contacted. See [Client.URL].
func URL(id EndpointID) (string, error) { return Default().URL(id) }

// EnableDownloads grants file-download consent on [Default].
// See [Client.EnableDownloads].
func EnableDownloads(maxSize int64, ids ...EndpointID) { Default().EnableDownloads(maxSize, ids...) }

// DisableDownloads revokes file-download consent on [Default].
// See [Client.DisableDownloads].
func DisableDownloads(ids ...EndpointID) { Default().DisableDownloads(ids...) }

// DownloadsEnabled reports [Default]'s consent state for id.
// See [Client.DownloadsEnabled].
func DownloadsEnabled(id EndpointID) (ok bool, maxSize int64) { return Default().DownloadsEnabled(id) }

// SetPolicy installs a custom download-consent policy on [Default].
// See [Client.SetPolicy].
func SetPolicy(p Policy) { Default().SetPolicy(p) }

// CheckDownload applies [Default]'s consent configuration to a prospective
// download. See [Client.CheckDownload].
func CheckDownload(id EndpointID, name string, size int64) error {
	return Default().CheckDownload(id, name, size)
}
