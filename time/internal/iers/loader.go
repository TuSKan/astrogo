package iers

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
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

// FSLoader is a [Loader] reading one finals2000A object from a filesystem,
// using nothing but the standard library.
//
// It serves the deployment that pre-seeds EOP data and wants no network
// dependency: astrogo/remote/eop's loader offers the same cache read, but
// linking it costs net/http, crypto/tls and resty. The filesystem is the
// caller's, which is what keeps an OS path out of astrogo's API: a directory
// on disk is os.DirFS(dir), written at the caller's own call site, and a
// remote.FS works unchanged. Fetch always reports [ErrNoEOPData] — reading
// what is already there is not a download.
//
// It used to take an OS path, as a string, and read it with os.ReadFile
// (#509).
type FSLoader struct {
	// FS holds the bulletin.
	FS fs.FS
	// Name is the bulletin's name within FS, "/"-separated, e.g.
	// "finals2000A.data".
	Name string
}

// errNoFS is a zero-value FSLoader: a misconfiguration, not an absence.
var errNoFS = errors.New("iers: FSLoader has no filesystem")

// Cached reads the object.
//
// A missing object is [ErrNoEOPData]: "nothing pre-seeded here" is an
// ordinary state. Any other failure — a permission error, an unreadable
// mount — is returned as itself. It used to be folded into ErrNoEOPData too,
// which made a deployment whose pre-seeded file could not be read look
// exactly like one that had never seeded it.
func (f FSLoader) Cached(_ context.Context) (Data, error) {
	if f.FS == nil {
		return Data{}, errNoFS
	}

	raw, err := fs.ReadFile(f.FS, f.Name)
	if errors.Is(err, fs.ErrNotExist) {
		return Data{}, ErrNoEOPData
	}

	if err != nil {
		return Data{}, fmt.Errorf("iers: read %s: %w", f.Name, err)
	}

	var mod time.Time
	if info, serr := fs.Stat(f.FS, f.Name); serr == nil {
		mod = info.ModTime()
	}

	return Data{Raw: raw, ModTime: mod}, nil
}

// Fetch always reports [ErrNoEOPData]: an FSLoader downloads nothing.
func (FSLoader) Fetch(context.Context) (Data, error) { return Data{}, ErrNoEOPData }
