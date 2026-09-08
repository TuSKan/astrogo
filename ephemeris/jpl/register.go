package jpl

import (
	"context"
	"fmt"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl/spk"
)

// Importing this package registers it as the backend the root ephemeris
// package's kernel-backed sources use:
//
//	import _ "github.com/TuSKan/astrogo/ephemeris/jpl"
//
// That is the whole reason a blank import means anything here. Without it,
// eph.NewProvider(ctx, eph.Planets, ...) reports [core.ErrNoKernelBackend] and
// names this import; with it, the root package costs what a kernel-backed
// build actually needs — this package reaches remote, which reaches
// gocloud.dev/blob, and that is about 12 MB of client a pure-SOFA call has no
// use for. See ephemeris/core/kernel.go and #112.
//
// A caller who imports this package for its own API — jpl.NewProvider,
// jpl.Provider — gets the registration too, and wants it: the two are the same
// capability.
//
// The lint exemption is narrow and the reason is the paragraph above: this
// package's entire purpose under a blank import is the registration, so the
// init is the contract rather than a hidden side effect. CLAUDE.md's rule
// forbids the latter, and an exported Register() the caller had to remember to
// invoke would make a blank import do nothing — which is the one behaviour a
// reader of `import _` will not expect.
//
//nolint:gochecknoinits // the registration is this package's documented purpose under a blank import
func init() { core.RegisterKernelBackend(build) }

// build is the kernel-backed half of eph.NewProvider, moved here so the root
// package no longer names this one.
func build(ctx context.Context, req core.KernelRequest) (core.Provider, error) {
	var opts []Option

	if !req.Start.IsZero() && !req.End.IsZero() {
		opts = append(opts, WithTimeInterval(req.Start, req.End))
	}

	p, err := NewProvider(ctx, req.Source, req.Kernel, opts...)
	if err != nil {
		return nil, fmt.Errorf("ephemeris: new provider: %w", err)
	}

	for _, extra := range req.ExtraKernels {
		k, err := spk.CacheDownload(ctx, "planets/"+extra+".bsp")
		if err != nil {
			return nil, fmt.Errorf("ephemeris: cache kernel %s: %w", extra, err)
		}

		if err := p.AddKernel(k); err != nil {
			return nil, fmt.Errorf("ephemeris: add kernel: %w", err)
		}
	}

	return p, nil
}
