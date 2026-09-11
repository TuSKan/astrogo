package remote

import (
	"sync"
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
	return &Client{endpoints: defaultEndpoints()}
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
