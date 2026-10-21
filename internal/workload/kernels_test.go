//go:build integration

package workload_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/ephemeris/jpl/spk"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

// The cache names jpl.NewProvider gives the planetary kernel and its
// leap-second file.
const (
	planetsKernel = "planets/de440s.bsp"
	lskKernel     = "lsk/naif0012.tls"
)

// kernelImage is de440s and its leap-second file, read into memory once per
// test binary so that every backend below serves byte-identical data.
type kernelImage struct {
	spk, lsk []byte
	mem      remote.FS // both files, on a mem:// filesystem
}

var loadImage = sync.OnceValues(func() (kernelImage, error) {
	ctx := context.Background()

	// Through the library's own constructor first: it is what downloads the
	// kernel on a machine without one and checks it against its recorded
	// checksum, so the bytes read below are the ones a caller evaluates.
	p, err := jpl.NewProvider(ctx, core.Planets, "de440s")
	if err != nil {
		return kernelImage{}, fmt.Errorf("de440s: %w", err)
	}

	if err := p.Close(); err != nil {
		return kernelImage{}, fmt.Errorf("de440s: close: %w", err)
	}

	var img kernelImage

	if img.spk, err = readCached(ctx, remote.NAIFSPK, planetsKernel); err != nil {
		return kernelImage{}, err
	}

	if img.lsk, err = readCached(ctx, remote.NAIFLSK, lskKernel); err != nil {
		return kernelImage{}, err
	}

	if img.mem, err = remote.OpenFS(ctx, "mem://workload"); err != nil {
		return kernelImage{}, fmt.Errorf("mem: %w", err)
	}

	for name, data := range map[string][]byte{planetsKernel: img.spk, lskKernel: img.lsk} {
		if err := remote.WriteFile(ctx, img.mem, name, bytes.NewReader(data)); err != nil {
			return kernelImage{}, fmt.Errorf("mem: write %s: %w", name, err)
		}
	}

	return img, nil
})

func readCached(ctx context.Context, id remote.EndpointID, name string) ([]byte, error) {
	fsys, key, err := remote.GetFile(ctx, id, name, remote.WithCacheName(name))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}

	data, err := fs.ReadFile(fsys, key)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}

	return data, nil
}

func mustImage(tb testing.TB) kernelImage {
	tb.Helper()

	img, err := loadImage()
	if err != nil {
		tb.Fatalf("kernel image: %v", err)
	}

	return img
}

// backend is one way of holding the same kernel. open returns the kernel's
// read counter when the backend has one, and nil when it does not.
type backend struct {
	name string
	open func(tb testing.TB) (*jpl.Provider, *readCounter)
}

// backends separates what reading a kernel costs from what evaluating it
// costs:
//
//	disk   jpl.NewProvider, as a caller gets it: the cached file read through
//	       remote.Open's chunk cache
//	mem    the same chunk cache over a mem:// copy: the cache without the
//	       system calls beneath it
//	bytes  a ReaderAt over a byte slice: no cache and no I/O, so what is left
//	       is segment lookup, decoding and Chebyshev evaluation
var backends = []backend{
	{"disk", openDisk},
	{"mem", openMem},
	{"bytes", openCounted},
}

// openDisk has no read counter; its signature is a backend's.
func openDisk(tb testing.TB) (*jpl.Provider, *readCounter) {
	tb.Helper()
	mustImage(tb) // the download and checksum, outside anything timed

	p, err := jpl.NewProvider(context.Background(), core.Planets, "de440s")
	if err != nil {
		tb.Fatalf("de440s: %v", err)
	}

	tb.Cleanup(func() { _ = p.Close() })

	return p, nil
}

// openMem has no read counter; its signature is a backend's.
func openMem(tb testing.TB) (*jpl.Provider, *readCounter) {
	tb.Helper()

	img := mustImage(tb)

	p, err := jpl.Open(context.Background(), img.mem, lskKernel, planetsKernel)
	if err != nil {
		tb.Fatalf("de440s on mem://: %v", err)
	}

	tb.Cleanup(func() { _ = p.Close() })

	return p, nil
}

// openCounted is the bytes backend, with its reads counted.
func openCounted(tb testing.TB) (*jpl.Provider, *readCounter) {
	tb.Helper()

	img := mustImage(tb)

	p, err := jpl.Open(context.Background(), img.mem, lskKernel)
	if err != nil {
		tb.Fatalf("leap seconds: %v", err)
	}

	tb.Cleanup(func() { _ = p.Close() })

	c := new(readCounter)
	c.add(tb, p, memFile{bytes.NewReader(img.spk)}, "de440s")

	return p, c
}

// readCounter counts the reads issued by every kernel of one provider.
//
// Before the record cache (#802) a type 2 or 3 evaluation read twice — the
// segment's directory, then the record — so half the count was the number of
// segment evaluations, and the baseline in docs/ephemeris.md §4 reports it as
// evals/op. Since then a segment's directory is read once and a record once
// for as long as evaluations stay in it, so the count is reported as what it
// is, reads/op.
type readCounter struct {
	reads atomic.Int64
}

// add puts the kernel read from ra into p, counting its reads.
func (c *readCounter) add(tb testing.TB, p *jpl.Provider, ra spk.ReadAtCloser, name string) {
	tb.Helper()

	r, err := spk.NewReader(countedFile{ra, c})
	if err != nil {
		tb.Fatalf("%s: %v", name, err)
	}

	if err := p.AddKernel(r); err != nil {
		tb.Fatalf("%s: %v", name, err)
	}
}

// countedFile is one kernel whose reads go to a shared counter.
type countedFile struct {
	spk.ReadAtCloser

	c *readCounter
}

func (f countedFile) ReadAt(p []byte, off int64) (int, error) {
	f.c.reads.Add(1)

	return f.ReadAtCloser.ReadAt(p, off) //nolint:wrapcheck // a pass-through: the kernel's own error is the one to report
}

// memFile is a kernel held in memory.
type memFile struct {
	*bytes.Reader
}

func (memFile) Close() error { return nil }

// countingProvider counts State calls, the unit every consumer above the
// provider spends: a light-time iteration, a root finder's step, a target in
// a scheduler.
type countingProvider struct {
	core.Provider

	calls atomic.Int64
}

func (c *countingProvider) Locate(id core.ID, t time.Time) (core.State, error) {
	c.calls.Add(1)

	return c.Provider.Locate(id, t) //nolint:wrapcheck // a pass-through: the provider's own error is the one to report
}

// openCachedKernel adds an SPK kernel to p, counting its reads on c, only if
// a previous consented run left it in the cache, and skips otherwise. The
// satellite kernels are a hundred megabytes and more, and a benchmark is no
// reason to fetch one.
func openCachedKernel(tb testing.TB, p *jpl.Provider, c *readCounter, name string) {
	tb.Helper()

	ctx := context.Background()

	fsys, prefix, err := remote.CacheDir(ctx, remote.NAIFSPK)
	if err != nil {
		tb.Fatalf("kernel cache: %v", err)
	}

	key := prefix + name

	if _, err := fs.Stat(fsys, key); errors.Is(err, fs.ErrNotExist) {
		tb.Skipf("%s is not cached, and this suite does not download it: %v", name, err)
	} else if err != nil {
		tb.Fatalf("%s: %v", name, err)
	}

	ra, err := remote.Open(ctx, fsys, key)
	if err != nil {
		tb.Fatalf("%s: %v", name, err)
	}

	c.add(tb, p, ra, name)
}
