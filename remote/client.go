package remote

import (
	"sync"
)

// Client is one astrogo I/O policy: which endpoints may be contacted, which
// may download and up to what size, whether to go to the network at all, and
// where what is fetched is cached.
//
// # Why this is a value rather than package state
//
// Because a policy is not a property of the process. Two components in one
// binary legitimately want different ones — an HTTP handler that must never
// block on a download beside a background prefetcher whose whole job is to
// download — and while this package held one set of globals, the second could
// not exist without changing the first. [SetOffline] stopped the prefetcher
// too.
//
// It is also what made the consent gate awkward to test: a package's TestMain
// granted downloads for the entire binary because there was no narrower scope
// to grant them in.
//
// # What it holds, and what it does not
//
// Exactly the four things [Capture] already snapshots, which is not a
// coincidence — that type was the existing admission that these four travel
// together and are what a caller means by "the configuration".
//
//   - the endpoint table, with each entry's enabled flag, URL override and
//     download consent;
//   - offline mode;
//   - the custom download [Policy], if any;
//   - the data directory URL.
//
// What it does not hold is the *description* of the world: an endpoint's kind,
// subsystem, timeouts, approximate size and default URL are facts about the
// service, identical for every client, and [Endpoints] reports them. A Client
// starts from that description and layers policy on it.
//
// # The default
//
// Every package-level function in this package operates on [Default], exactly
// as [net/http.Get] operates on [net/http.DefaultClient]. Code that has never
// heard of a Client keeps working, and a caller who wants one policy for the
// process still uses [EnableDownloads] and friends.
//
// A Client is safe for concurrent use.
type Client struct {
	mu        sync.RWMutex
	endpoints map[EndpointID]Endpoint
	offline   bool
	policy    Policy
	dataDir   string
}

// defaultClient backs every package-level function here.
//
// It is a var rather than a func so [Default] is a plain read: the package
// functions go through it on every call, and the endpoint registry is the
// hottest read path in this package.
var defaultClient = NewClient()

// Default returns the Client the package-level functions operate on.
//
// It is the [net/http.DefaultClient] of this package: shared, mutable through
// [SetOffline], [EnableDownloads], [SetDataDir] and the rest, and the right
// thing for a program with one policy. A program with two makes its own with
// [NewClient] rather than fighting over this one.
func Default() *Client { return defaultClient }

// ClientOption configures a [Client] at construction.
type ClientOption func(*Client)

// NewClient returns a Client with astrogo's default policy: every endpoint at
// its built-in URL and enabled, downloads denied, online, and the data
// directory unset so it resolves from ASTROGO_CACHE_DIR or the OS cache
// directory when first needed.
//
// Downloads are denied because that is astrogo's standing rule — nothing is
// ever fetched without being asked for — and a new Client is not a way around
// it. [WithDownloads] is.
func NewClient(opts ...ClientOption) *Client {
	c := &Client{endpoints: defaultEndpoints()}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// WithOffline starts the Client in offline mode, where every endpoint access —
// API call or download — fails with [ErrOffline].
func WithOffline(off bool) ClientOption {
	return func(c *Client) { c.offline = off }
}

// WithDownloads grants file-download consent at construction, with the
// semantics of [Client.EnableDownloads]: maxSize caps a single download (0 is
// unlimited), and no ids means every Downloadable endpoint.
func WithDownloads(maxSize int64, ids ...EndpointID) ClientOption {
	return func(c *Client) { c.setConsent(true, maxSize, ids) }
}

// WithCacheURL sets the base location for everything the Client stores, as any
// bucket URL remote/file can open. See [Client.SetDataDir].
func WithCacheURL(bucketURL string) ClientOption {
	return func(c *Client) { c.dataDir = bucketURL }
}

// WithEndpointURL overrides one endpoint's base URL — a mirror, an internal
// proxy, or an httptest server in a test. Unlike the package-level [SetURL] it
// cannot report an unknown id, since an option returns nothing; an id that is
// not registered is ignored, and [Client.SetURL] is the checked form.
func WithEndpointURL(id EndpointID, url string) ClientOption {
	return func(c *Client) {
		if ep, ok := c.endpoints[id]; ok {
			ep.URL = url
			c.endpoints[id] = ep
		}
	}
}

// WithPolicy installs a custom download-consent policy, replacing the
// per-endpoint consent checks entirely. See [Policy].
func WithPolicy(p Policy) ClientOption {
	return func(c *Client) { c.policy = p }
}
