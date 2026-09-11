package remote

import (
	"slices"
	"sync"

	"github.com/TuSKan/astrogo/remote/api"
)

// Client is one component's I/O policy: which endpoints it may reach, where
// they live, what it has consented to download, and where it caches.
//
// # Why this exists
//
// The package-level [SetOffline], [EnableDownloads], [SetURL] and [SetDataDir]
// configure one policy for a whole process, which is fine for a program and
// wrong for a service. Two components in one binary — an HTTP handler serving
// user queries and a background kernel prefetcher — cannot hold different
// policies, so the handler cannot be offline-only while the prefetcher
// downloads, which is exactly how such a service would want to be configured
// (#114).
//
// A Client is that policy as a value. Build one, configure it, pass it to the
// component that owns it:
//
//	prefetch := remote.NewClient()
//	prefetch.EnableDownloads(200<<20, remote.NAIFSPK)
//
//	serve := remote.NewClient()
//	serve.SetOffline(true)
//
// Nothing the prefetcher grants reaches the handler.
//
// # The package-level functions are still there
//
// They operate on [Default], the way net/http's package-level Get operates on
// http.DefaultClient. A program that does not care about scoping writes exactly
// what it wrote before and behaves exactly as before; [Default] is not special
// beyond being the one the package functions name.
//
// # What a Client does not hold
//
// The endpoint registry is a description of the world — what SIMBAD is, that a
// kernel is roughly 120 MB, that Horizons answers JSON — not a policy, so every
// Client starts from the same built-in description and differs only in what it
// then decides about it. There is no way to register an endpoint that some
// clients can see and others cannot, because an endpoint nobody described is
// not a policy question.
//
// Safe for concurrent use.
type Client struct {
	// endpoints is this client's view: the built-in description, plus
	// whatever this client has since decided about each entry — a URL
	// override, a disable, a download grant.
	//
	// A copy rather than a shared registry with a per-client overlay beside
	// it, because the copy is about twenty small structs and the overlay
	// would be a second place every read has to consult and every reader has
	// to remember to consult. The cost is a map clone per client; clients are
	// built once per component, not per request.
	endpoints map[EndpointID]Endpoint

	// policy replaces the per-endpoint consent checks entirely when set.
	policy Policy

	// dataDirURL is where this client caches. Empty means "resolve the
	// default lazily" — see DataDirURL, which reads the environment on every
	// call and so must not be resolved once at construction.
	dataDirURL string

	// transports is one HTTP client per endpoint this client has issued a
	// request to, each built with that endpoint's registered timeout and this
	// client's apiOpts.
	//
	// Per endpoint because the two things a transport carries are properties
	// of the service rather than of the caller: a timeout belongs to whoever
	// has to answer within it, and pacing exists to be polite to a particular
	// service. Built lazily, so a client that never calls an API opens
	// nothing.
	transports map[EndpointID]*api.Client

	// apiOpts configure every transport this client builds. See
	// [Client.SetAPIOptions].
	apiOpts []APIOption

	// offline cuts every endpoint access at the gate.
	offline bool

	mu sync.RWMutex
}

// NewClient returns a client with the default policy: online, every endpoint
// enabled at its built-in URL, no download consent, and the default cache
// location.
//
// It is the same starting state the process-wide default has, so a component
// that builds its own client is opting out of whatever else the program has
// configured rather than inheriting it. Inheriting would be the more
// surprising of the two: the reason to build a second client is that the first
// one's policy is not yours.
func NewClient() *Client {
	return &Client{
		endpoints:  defaultEndpoints(),
		transports: make(map[EndpointID]*api.Client),
	}
}

// Clone returns a client holding a copy of this one's policy and its own
// connections.
//
// It is how a component takes the program's configuration without taking its
// connections or imposing its own settings back on it: the copy starts offline
// if this client is offline and with whatever consent it has been granted, and
// then diverges. Changing either afterwards leaves the other alone.
//
// The alternative — [NewClient] — starts from the built-in defaults instead,
// which for a component running inside a configured program means silently
// ignoring that configuration, including its offline mode.
func (c *Client) Clone() *Client {
	c.mu.RLock()
	defer c.mu.RUnlock()

	endpoints := make(map[EndpointID]Endpoint, len(c.endpoints))
	for id, ep := range c.endpoints {
		endpoints[id] = cloneEndpoint(ep)
	}

	return &Client{
		endpoints:  endpoints,
		transports: make(map[EndpointID]*api.Client),
		apiOpts:    slices.Clone(c.apiOpts),
		policy:     c.policy,
		dataDirURL: c.dataDirURL,
		offline:    c.offline,
	}
}

// SetAPIOptions configures every transport this client builds from now on:
// timeouts, retry policy, pacing, a bearer token.
//
// It replaces rather than appends, and it discards any transport already
// built, so the next request opens a fresh one under the new settings. A
// caller that set an option would otherwise have to guess which of its
// requests had already established a connection.
//
// These are per client rather than per call because they describe a
// relationship with a service — how long to wait for it, how hard to press it,
// who we are to it — and a program that needs two such relationships builds
// two clients. [Client.Clone] is how to get the second one without reinventing
// the first one's policy.
func (c *Client) SetAPIOptions(opts ...APIOption) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, inner := range c.transports {
		_ = inner.Close()
	}

	clear(c.transports)

	c.apiOpts = slices.Clone(opts)
}

// defaultClient backs every package-level function in this package.
//
// Not lazily initialised: defaultEndpoints() is a statically built map with no
// computation in it, so there is nothing to defer and no init() ordering to
// reason about.
var defaultClient = NewClient()

// Default returns the client the package-level functions operate on.
//
// It is the analogue of http.DefaultClient, and it is exported for the same
// reason: a caller holding a *Client generically — a helper that configures
// either the process default or a component's own — should not need two code
// paths.
func Default() *Client { return defaultClient }
