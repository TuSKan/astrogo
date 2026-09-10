package lsk

import "github.com/TuSKan/astrogo/remote"

// Option configures a leap-second kernel fetch. See [spk.Option] for why the
// shape is a variadic option rather than a parameter.
//
// [github.com/TuSKan/astrogo/ephemeris/jpl/spk.Option] is not reused here
// because that would make this package import spk for one type, inverting the
// dependency: an LSK is what an SPK's time scale is read against, so spk knows
// about lsk and not the other way round.
type Option func(*config)

type config struct{ remote *remote.Client }

// WithClient fetches under c's policy instead of [remote.Default]'s.
func WithClient(c *remote.Client) Option {
	return func(cfg *config) { cfg.remote = c }
}

// apply resolves opts to the client to fetch through, defaulting to
// [remote.Default].
func apply(opts []Option) *remote.Client {
	var cfg config

	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.remote == nil {
		return remote.Default()
	}

	return cfg.remote
}
