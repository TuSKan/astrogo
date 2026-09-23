package file_test

import (
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/remote/file"
)

// The benchmarks here answer one question, and it is a design question rather
// than a performance one: does an object opened through this package still need
// the chunk-caching [file.ReaderAt] wrapped around it?
//
// The answer differs by backend, which is why both are measured rather than one
// being taken as representative of the other. Measured on an i9-11980HK,
// 2000 SPK-shaped reads of 104 bytes over a 64 MiB object, -count=3:
//
//	                                        time/op    allocs/op
//	gocloud NewRangeReader per ReadAt      216 ms       94,067
//	gocloud chunked, 64 KiB × 16            53 µs            0
//	this package, file:// uncached         4.5 ms            0
//	this package, mem:// uncached           10 µs            0
//	this package, http uncached         10,780 ms      181,422
//	this package, chunked  4 KiB × 16      220 µs          142
//	this package, chunked 64 KiB × 16     48.5 µs            0   <- default
//	this package, chunked  1 MiB × 16     48.9 µs            0
//
// The gocloud rows are what BenchmarkReadAtStrategies measured before that
// stack was removed; the generator and the shape of the work are unchanged, so
// they are comparable with the rest.
//
// Three conclusions, all load-bearing.
//
// Dropping gocloud makes an uncached local ReadAt 48× cheaper and removes every
// allocation, because [localFS.Open] returns an *os.File and ReadAt is one
// positional read — where gocloud's NewRangeReader opened the file again per
// call.
//
// The chunk cache still earns its keep, so [Open] puts one in front of every
// file rather than offering it as an option. Locally it is a further 92×; over
// HTTP it is the difference between working and not, since a per-call ranged
// GET is 2000 requests for 2000 reads and takes eleven seconds against a server
// on localhost — a real endpoint adds a round trip to each one.
//
// 64 KiB stays the default chunk size. 1 MiB costs sixteen times the memory and
// is not faster, and 4 KiB is 4.5× slower and allocates, because a 104-byte
// read that straddles a boundary needs two chunks often enough to matter.
//
// CLAUDE.md's instruction not to simplify this away without re-running the
// benchmark stands; this is that re-run.

// spkPattern is the access pattern SPK segment evaluation produces: many small
// reads, clustered inside a segment, jumping between segments. It is the same
// generator the pre-migration BenchmarkReadAtStrategies used, so its recorded
// numbers in the table above are comparable with these.
func spkPattern(size int64, n int) []int64 {
	offs := make([]int64, 0, n)
	regions := []int64{0, size / 3, (size * 2) / 3}

	for i := range n {
		base := regions[i%len(regions)]

		off := base + int64((i/len(regions))%2048)*104
		if off+104 > size {
			off = base
		}

		offs = append(offs, off)
	}

	return offs
}

// benchReads is the number of reads every benchmark here performs, matching
// what the pre-migration BenchmarkReadAtStrategies used so the recorded figures
// stay comparable.
const benchReads = 2000

// benchReadAt runs the pattern against one open file.
func benchReadAt(b *testing.B, f file.File, size int64) {
	b.Helper()

	offs := spkPattern(size, benchReads)
	buf := make([]byte, 104)

	b.ResetTimer()

	for b.Loop() {
		for _, off := range offs {
			if _, err := f.ReadAt(buf, off); err != nil {
				b.Fatalf("ReadAt(%d): %v", off, err)
			}
		}
	}
}

// openOne opens a name and asserts it is byte-addressable, which is the whole
// contract [file.File] adds over [fs.File].
func openOne(b *testing.B, fsys fs.FS, name string) file.File {
	b.Helper()

	f, err := fsys.Open(name)
	if err != nil {
		b.Fatalf("Open %s: %v", name, err)
	}

	b.Cleanup(func() { _ = f.Close() })

	rf, ok := f.(file.File)
	if !ok {
		b.Fatalf("%T does not implement file.File, so it cannot be read at", f)
	}

	return rf
}

const benchSizeMB = 64

func benchObject(sizeMB int) []byte {
	data := make([]byte, sizeMB<<20)
	for i := range data {
		data[i] = byte(i)
	}

	return data
}

// BenchmarkLocalReadAt measures the local backend, where Open returns an
// *os.File and ReadAt is therefore a single positional read rather than an open
// followed by a seek.
func BenchmarkLocalReadAt(b *testing.B) {
	dir := b.TempDir()

	const name = "kernel.bsp"

	data := benchObject(benchSizeMB)
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		b.Fatal(err)
	}

	slash := filepath.ToSlash(dir)
	if !strings.HasPrefix(slash, "/") {
		slash = "/" + slash
	}

	u := url.URL{Scheme: "file", Path: slash}

	fsys, err := file.OpenFS(u.String())
	if err != nil {
		b.Fatalf("OpenFS: %v", err)
	}

	benchReadAt(b, openOne(b, fsys, name), int64(len(data)))
}

// BenchmarkHTTPReadAt measures the HTTP backend, where every ReadAt is its own
// ranged GET. The server is local, so this is the floor: a real endpoint adds
// the round trip.
//
// The request count is reported per operation. That number, not the time, is
// the one that decides whether the chunk cache survives — 2000 requests for
// 2000 reads is the behavior the cache exists to prevent, and a local server
// makes it look far cheaper than it is over a network.
func BenchmarkHTTPReadAt(b *testing.B) {
	data := benchObject(benchSizeMB)

	var gets atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gets.Add(1)

		start, end := benchRange(r.Header.Get("Range"), len(data))

		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
		w.Header().Set("Content-Length", strconv.Itoa(end-start+1))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(data[start : end+1])
	}))
	b.Cleanup(srv.Close)

	fsys, err := file.OpenFS(srv.URL + "/kernels/")
	if err != nil {
		b.Fatalf("OpenFS: %v", err)
	}

	f := openOne(b, fsys, "kernel.bsp")

	gets.Store(0)
	benchReadAt(b, f, int64(len(data)))
	b.ReportMetric(float64(gets.Load())/float64(b.N), "requests/op")
}

func benchRange(v string, size int) (start, end int) {
	v = strings.TrimPrefix(v, "bytes=")

	before, after, _ := strings.Cut(v, "-")
	start, _ = strconv.Atoi(before)
	end = size - 1

	if after != "" {
		end, _ = strconv.Atoi(after)
	}

	if end >= size {
		end = size - 1
	}

	return start, end
}

// BenchmarkMemReadAt is the control: a *bytes.Reader with no I/O at all, so the
// other two can be read as "this much above free".
func BenchmarkMemReadAt(b *testing.B) {
	fsys, err := file.OpenFS("mem://bench")
	if err != nil {
		b.Fatal(err)
	}

	cfs, ok := fsys.(file.CreateFS)
	if !ok {
		b.Fatal("mem:// does not implement CreateFS")
	}

	w, err := cfs.Create("kernel.bsp")
	if err != nil {
		b.Fatal(err)
	}

	data := benchObject(benchSizeMB)
	if _, err := w.Write(data); err != nil {
		b.Fatal(err)
	}

	if err := w.Close(); err != nil {
		b.Fatal(err)
	}

	benchReadAt(b, openOne(b, fsys, "kernel.bsp"), int64(len(data)))
}

// BenchmarkSequentialRead covers the other access pattern — a whole-object read,
// which is what a download does — and exists to pin that the HTTP backend holds
// one body open across Read calls. A request count above 1 means it regressed
// into a request per buffer-full.
func BenchmarkSequentialRead(b *testing.B) {
	const smallMB = 4

	data := benchObject(smallMB)

	var gets atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gets.Add(1)

		start, end := benchRange(r.Header.Get("Range"), len(data))

		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
		w.Header().Set("Content-Length", strconv.Itoa(end-start+1))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(data[start : end+1])
	}))
	b.Cleanup(srv.Close)

	fsys, err := file.OpenFS(srv.URL + "/kernels/")
	if err != nil {
		b.Fatal(err)
	}

	gets.Store(0)

	for b.Loop() {
		f, oerr := fsys.Open("kernel.bsp")
		if oerr != nil {
			b.Fatal(oerr)
		}

		n, cerr := io.Copy(io.Discard, f)
		_ = f.Close()

		if cerr != nil {
			b.Fatal(cerr)
		}

		if n != int64(len(data)) {
			b.Fatalf("read %d bytes, want %d", n, len(data))
		}
	}

	// One probe plus one body per iteration is the design; more means Read
	// stopped reusing its body.
	b.ReportMetric(float64(gets.Load())/float64(b.N), "requests/op")
}

// BenchmarkChunkedReadAt is the other half of the comparison: the same pattern
// through [file.Open], which puts the chunk cache in front of the backend.
//
// Together with BenchmarkLocalReadAt and BenchmarkHTTPReadAt this is what
// BenchmarkReadAtStrategies used to measure before gocloud went. It is kept
// because CLAUDE.md requires the chunked reader not be simplified away without
// re-running it, and a benchmark against a deleted stack cannot answer that.
func BenchmarkChunkedReadAt(b *testing.B) {
	dir := b.TempDir()

	const name = "kernel.bsp"

	data := benchObject(benchSizeMB)
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		b.Fatal(err)
	}

	slash := filepath.ToSlash(dir)
	if !strings.HasPrefix(slash, "/") {
		slash = "/" + slash
	}

	fsys, err := file.OpenFS((&url.URL{Scheme: "file", Path: slash}).String())
	if err != nil {
		b.Fatalf("OpenFS: %v", err)
	}

	for _, chunk := range []int64{4 << 10, 64 << 10, 1 << 20} {
		b.Run(fmt.Sprintf("%dKiB", chunk>>10), func(b *testing.B) {
			f, oerr := file.Open(b.Context(), fsys, name,
				file.WithChunkSize(chunk), file.WithCachedChunks(16))
			if oerr != nil {
				b.Fatal(oerr)
			}

			b.Cleanup(func() { _ = f.Close() })
			b.ReportMetric(float64(chunk*16)/(1<<20), "MiB-resident")
			benchReadAt(b, f, int64(len(data)))
		})
	}
}
