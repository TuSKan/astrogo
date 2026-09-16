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
//	this package, file:// (*os.File)       4.5 ms            0
//	this package, mem:// (bytes.Reader)     11 µs            0
//	this package, http (per-call GET)   11,400 ms      181,749
//
// Two conclusions, both load-bearing for the migration.
//
// Dropping gocloud makes an uncached local ReadAt 48× cheaper and removes every
// allocation, because [localFS.Open] returns an *os.File and ReadAt is one
// positional read — where gocloud's NewRangeReader opened the file again per
// call. That is a real gain and it is not the whole story.
//
// The chunk cache still earns its keep. Locally it is a further 84×; over HTTP
// it is the difference between working and not, since a per-call ranged GET is
// 2000 requests for 2000 reads and takes 11 seconds against a server on
// localhost — a real endpoint adds a round trip to each one. So [file.ReaderAt]
// survives the migration and is reshaped to wrap an fs.File rather than deleted.
// CLAUDE.md's instruction not to simplify it away without re-running the
// benchmark stands; this is that re-run.

// spkPattern is the access pattern SPK segment evaluation produces: many small
// reads, clustered inside a segment, jumping between segments. It is the same
// generator BenchmarkReadAtStrategies uses, so the two are comparable.
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

// benchReadAt runs the pattern against one open file.
func benchReadAt(b *testing.B, f file.File, size int64, reads int) {
	b.Helper()

	offs := spkPattern(size, reads)
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

	benchReadAt(b, openOne(b, fsys, name), int64(len(data)), 2000)
}

// BenchmarkHTTPReadAt measures the HTTP backend, where every ReadAt is its own
// ranged GET. The server is local, so this is the floor: a real endpoint adds
// the round trip.
//
// The request count is reported per operation. That number, not the time, is
// the one that decides whether the chunk cache survives — 2000 requests for
// 2000 reads is the behaviour the cache exists to prevent, and a local server
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
	benchReadAt(b, f, int64(len(data)), 2000)
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

	benchReadAt(b, openOne(b, fsys, "kernel.bsp"), int64(len(data)), 2000)
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
