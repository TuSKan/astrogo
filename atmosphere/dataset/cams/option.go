package cams

import "github.com/TuSKan/astrogo/remote"

// Option configures a CAMS fetch.
//
// It is defined here rather than borrowed from catalog/resolve because
// atmosphere is a scientific engine and catalog is orchestration: the
// architecture in CLAUDE.md has the dependency running the other way, and one
// option type is not worth inverting it.
type Option func(*config)

type config struct{ remote *remote.Client }

// WithClient fetches under c's policy instead of [remote.Default]'s — its
// download consent, offline flag and endpoint overrides.
//
// CAMS is a registration-gated endpoint whose files are ~1.5 MB per hour, so
// "may this component fetch" is a question a service is likely to answer
// differently for a request path and a warm-up job.
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
