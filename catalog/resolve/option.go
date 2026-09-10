package resolve

import (
	"github.com/TuSKan/astrogo/remote"
)

// Option configures a catalog provider at construction.
//
// # Why it lives here rather than in each provider
//
// Because there is one thing to configure and eight providers that need it,
// and eight identical Option types would be eight times the API surface for
// one capability. Every provider in catalog/ already imports this package for
// [Provider] and [NewMapCache], so this costs no new dependency.
//
// The cost is that a caller writes resolve.WithClient rather than
// simbad.WithClient. That is the right trade at this size: the alternative
// reads marginally better at each call site and adds seven exported types to a
// library that is about to freeze its shape.
type Option func(*config)

// config is what the options build. Unexported: it is plumbing, and a caller
// has no reason to construct one.
type config struct{ remote *remote.Client }

// WithClient binds a provider to one [remote.Client]'s policy instead of
// [remote.Default] — its offline flag, endpoint overrides and enabled set.
//
// This is what lets two components in one binary hold different policies: an
// HTTP handler that must never touch the network, and a prefetcher that may.
//
//	sim := simbad.New(resolve.WithClient(offlineOnly))
func WithClient(c *remote.Client) Option {
	return func(cfg *config) { cfg.remote = c }
}

// ClientOf is the client opts selected, or nil for none.
//
// Nil rather than [remote.Default] on purpose: every consumer passes the result
// straight to [api.WithRemote], which already treats nil as the default, so
// resolving it here would be a second place that decides the same thing. One
// accessor, one meaning.
//
//	api.NewClient(remote.SIMBAD, api.WithRemote(resolve.ClientOf(opts)))
func ClientOf(opts []Option) *remote.Client {
	var cfg config

	for _, opt := range opts {
		opt(&cfg)
	}

	return cfg.remote
}
