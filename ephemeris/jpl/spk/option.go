package spk

import "github.com/TuSKan/astrogo/remote"

// Option configures a kernel fetch.
//
// It exists so [CacheDownload] and [CacheAPI] can be told which
// [remote.Client]'s policy to fetch under without changing their signatures.
// A niladic call site keeps compiling and keeps using [remote.Default], which
// is what makes #114 a shape change rather than a break.
type Option func(*config)

type config struct{ remote *remote.Client }

// WithClient fetches under c's policy instead of [remote.Default]'s — its
// download consent, offline flag and endpoint overrides.
//
// This is the option that matters most of the set. Kernels are the largest
// thing astrogo downloads, so "may this component fetch, and up to what size"
// is a question most likely to have two answers in one binary.
func WithClient(c *remote.Client) Option {
	return func(cfg *config) { cfg.remote = c }
}

// clientFrom resolves opts to the client to fetch through, defaulting to
// [remote.Default] so a caller who asked for nothing gets exactly the
// behaviour they had before this existed.
func clientFrom(opts []Option) *remote.Client {
	var cfg config

	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.remote == nil {
		return remote.Default()
	}

	return cfg.remote
}
