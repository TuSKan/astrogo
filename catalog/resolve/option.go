package resolve

import (
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/remote/api"
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
type Option func(*Config)

// Config is what the options build. A provider reads it once in its
// constructor; it is exported only because the providers are separate
// packages.
type Config struct {
	// Remote is the policy endpoint resolution goes through. Nil means
	// [remote.Default], which is what every caller who has not asked for
	// anything else gets.
	Remote *remote.Client
}

// WithClient binds a provider to one [remote.Client]'s policy instead of
// [remote.Default] — its offline flag, endpoint overrides and enabled set.
//
// This is what lets two components in one binary hold different policies: an
// HTTP handler that must never touch the network, and a prefetcher that may.
//
//	sim := simbad.New(resolve.WithClient(offlineOnly))
func WithClient(c *remote.Client) Option {
	return func(cfg *Config) { cfg.Remote = c }
}

// Apply builds a Config from opts. Providers call it as the first line of
// their constructor.
func Apply(opts []Option) Config {
	var cfg Config

	for _, opt := range opts {
		opt(&cfg)
	}

	return cfg
}

// APIOptions translates a Config into the options [api.NewClient] takes.
//
// It returns nil for a Config that asked for nothing, so a provider built
// without options constructs exactly the API client it did before — the
// default policy, reached by the default path, with no branch of its own.
func (c Config) APIOptions() []api.Option {
	if c.Remote == nil {
		return nil
	}

	return []api.Option{api.WithRemote(c.Remote)}
}

// RemoteOrDefault is the client a provider should fetch through.
//
// For providers that call [remote.Client.GetFile] rather than going through
// an API client, where there is no options slice to pass along and a nil
// receiver would panic rather than fall back.
func (c Config) RemoteOrDefault() *remote.Client {
	if c.Remote == nil {
		return remote.Default()
	}

	return c.Remote
}
