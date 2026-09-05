package iers

import (
	"context"
	"errors"
	"os"
	"sync"
	"time"
)

// ErrNoLoader is returned when EOP data is needed but no Loader has been
// registered.
//
// Importing astrogo/remote registers one automatically, which is what any
// program granting download consent already does. A program that imports
// neither gets this, and degrades to zero EOP exactly as an unconsented
// one does.
var ErrNoLoader = errors.New("iers: no EOP loader registered")

// ErrNoEOPData is returned by a Loader that found nothing to return —
// no cached file, or a fetch that consent forbade.
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

// Loader supplies raw EOP data. It exists so this package — and so
// astrogo/time above it — needs no knowledge of caches, HTTP, download
// consent or blob storage, and links none of it.
//
// The two methods are the two steps the lazy load has always taken, kept
// separate because they differ in more than where the bytes come from:
// Cached must never touch the network and never needs consent, while
// Fetch is consent-gated and may fail for that reason alone.
type Loader interface {
	// Cached returns EOP data already on disk, without network access and
	// without a consent check. Returns [ErrNoEOPData] when there is none.
	Cached(ctx context.Context) (Data, error)

	// Fetch downloads fresh EOP data, subject to download consent.
	Fetch(ctx context.Context) (Data, error)
}

var (
	loaderMu sync.RWMutex
	loader   Loader
)

// RegisterLoader sets the process-wide EOP loader. Passing nil unregisters,
// which is how a test restores the pristine state.
func RegisterLoader(l Loader) {
	loaderMu.Lock()
	defer loaderMu.Unlock()

	loader = l
}

// GetLoader returns the registered loader, or nil.
func GetLoader() Loader {
	loaderMu.RLock()
	defer loaderMu.RUnlock()

	return loader
}

// FileLoader is a [Loader] reading one finals2000A file from a fixed path,
// using nothing but the standard library.
//
// It serves the deployment that pre-seeds EOP data on disk and wants no
// cloud-storage dependency: astrogo/remote's loader offers the same cache
// read, but linking remote for it costs the whole blob-storage tree.
// Fetch always reports [ErrNoEOPData] — a fixed path is not a download.
type FileLoader string

// Cached reads the file. A missing file is [ErrNoEOPData], not an error
// worth propagating: "nothing pre-seeded here" is an ordinary state.
func (f FileLoader) Cached(_ context.Context) (Data, error) {
	raw, err := os.ReadFile(string(f))
	if err != nil {
		return Data{}, ErrNoEOPData
	}

	var mod time.Time
	if info, serr := os.Stat(string(f)); serr == nil {
		mod = info.ModTime()
	}

	return Data{Raw: raw, ModTime: mod}, nil
}

// Fetch always reports [ErrNoEOPData]: a FileLoader downloads nothing.
func (f FileLoader) Fetch(context.Context) (Data, error) { return Data{}, ErrNoEOPData }
