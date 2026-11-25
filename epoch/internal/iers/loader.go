package iers

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
)

// ErrNoEOPData is returned by a Loader that found nothing to return —
// no cached file, or a fetch the download policy refused.
var ErrNoEOPData = errors.New("iers: no EOP data available")

// Data is raw finals2000A content together with where it came from.
type Data struct {
	// Raw is the unparsed finals2000A bulletin.
	Raw []byte

	// ModTime is when this copy was written, used to seed the retry
	// cooldown so a fresh process does not immediately re-download after
	// a recent failed attempt. Zero if unknown.
	ModTime time.Time
}

// Loader supplies raw EOP data: the two steps the lazy load takes, kept
// separate because they differ in more than where the bytes come from.
// Cached must never touch the network and never asks the download policy,
// while Fetch asks it and may fail for that reason alone.
//
// The loader in use is [remoteLoader] unless [RegisterLoader] set another,
// which only this package's tests do, to count and script the two steps.
type Loader interface {
	// Cached returns EOP data already on disk, without network access and
	// without asking the download policy. Returns [ErrNoEOPData] when there
	// is none.
	Cached(ctx context.Context) (Data, error)

	// Fetch downloads fresh EOP data, subject to the download policy.
	Fetch(ctx context.Context) (Data, error)
}

// loader is the registered Loader, nil for the remote one. It is read
// without a lock, since every uncovered lookup asks for it (#953).
var loader atomic.Pointer[Loader]

// RegisterLoader replaces the loader the lazy load uses. Passing nil puts back
// the remote one.
func RegisterLoader(l Loader) {
	if l == nil {
		loader.Store(nil)

		return
	}

	loader.Store(&l)
}

// GetLoader returns the loader the lazy load uses: the registered one, or the
// remote one when none is.
func GetLoader() Loader {
	if l := loader.Load(); l != nil {
		return *l
	}

	return remoteLoader{}
}
