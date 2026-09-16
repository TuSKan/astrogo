package file_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/TuSKan/astrogo/remote/file"
)

// kernel is the object the HTTP tests serve: big enough that a range is worth
// taking, and patterned so a wrong offset is visible rather than plausible.
var kernel = func() []byte {
	b := make([]byte, 64*1024)
	for i := range b {
		b[i] = byte(i % 251) // a prime, so no alignment makes the pattern repeat
	}

	return b
}()

// objectServer serves one object with whatever range behaviour a test asks for.
type objectServer struct {
	allowHEAD     bool
	sendLength    bool
	sendRangeSize bool

	mu    sync.Mutex
	heads int
	gets  int
}

func (s *objectServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	switch r.Method {
	case http.MethodHead:
		s.heads++
	case http.MethodGet:
		s.gets++
	}
	s.mu.Unlock()

	if !strings.HasSuffix(r.URL.Path, "/de440s.bsp") {
		http.NotFound(w, r)

		return
	}

	if r.Method == http.MethodHead {
		if !s.allowHEAD {
			w.WriteHeader(http.StatusMethodNotAllowed)

			return
		}

		if s.sendLength {
			w.Header().Set("Content-Length", strconv.Itoa(len(kernel)))
		}

		w.WriteHeader(http.StatusOK)

		return
	}

	rng := r.Header.Get("Range")
	if rng == "" {
		_, _ = w.Write(kernel)

		return
	}

	start, end := parseRange(rng, len(kernel))

	if s.sendRangeSize {
		w.Header().Set("Content-Range",
			fmt.Sprintf("bytes %d-%d/%d", start, end, len(kernel)))
	}

	w.Header().Set("Content-Length", strconv.Itoa(end-start+1))
	w.WriteHeader(http.StatusPartialContent)
	_, _ = w.Write(kernel[start : end+1])
}

func parseRange(v string, size int) (start, end int) {
	v = strings.TrimPrefix(v, "bytes=")
	parts := strings.SplitN(v, "-", 2)
	start, _ = strconv.Atoi(parts[0])
	end = size - 1

	if len(parts) == 2 && parts[1] != "" {
		end, _ = strconv.Atoi(parts[1])
	}

	if end >= size {
		end = size - 1
	}

	return start, end
}

func serve(t *testing.T, s *objectServer) fs.FS {
	t.Helper()

	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)

	fsys, err := file.OpenFS(srv.URL + "/pub/naif/")
	if err != nil {
		t.Fatalf("OpenFS: %v", err)
	}

	return fsys
}

// TestHTTPReadsWholeAndRanged covers the two ways an object is read, and that
// they agree.
func TestHTTPReadsWholeAndRanged(t *testing.T) {
	t.Parallel()

	fsys := serve(t, &objectServer{allowHEAD: true, sendLength: true, sendRangeSize: true})

	f, err := fsys.Open("de440s.bsp")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}

	if info.Size() != int64(len(kernel)) {
		t.Errorf("Stat says %d bytes, the object is %d", info.Size(), len(kernel))
	}

	rf, ok := f.(file.File)
	if !ok {
		t.Fatal("an HTTP object does not implement file.File, so it cannot be seeked or read at")
	}

	// Sequential.
	whole, err := io.ReadAll(rf)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if string(whole) != string(kernel) {
		t.Errorf("sequential read returned %d bytes, want %d", len(whole), len(kernel))
	}

	// Ranged, from the middle, which is the access pattern an SPK kernel has.
	buf := make([]byte, 4096)

	n, err := rf.ReadAt(buf, 30000)
	if err != nil {
		t.Fatalf("ReadAt: %v", err)
	}

	if n != len(buf) || string(buf) != string(kernel[30000:30000+len(buf)]) {
		t.Errorf("ReadAt(30000) returned %d bytes and the wrong ones", n)
	}

	// ReadAt must not have moved the cursor, which is its contract and is what
	// lets remote's chunked reader share one file across goroutines.
	if pos, serr := rf.Seek(0, io.SeekCurrent); serr != nil || pos != int64(len(kernel)) {
		t.Errorf("after ReadAt the cursor is at %d (err %v), want %d — ReadAt moved it",
			pos, serr, len(kernel))
	}
}

// TestHTTPProbesSizeWithoutHEAD is the case that put the fallback in the old
// driver, transcribed here because a server that refuses HEAD is common enough
// to have cost somebody a day already.
func TestHTTPProbesSizeWithoutHEAD(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		srv  *objectServer
	}{
		{
			name: "HEAD refused, size from Content-Range",
			srv:  &objectServer{allowHEAD: false, sendRangeSize: true},
		},
		{
			// HEAD answers but says nothing about length — what a chunked
			// response looks like. "Unknown" must not be read as "empty".
			name: "HEAD answers without a length",
			srv:  &objectServer{allowHEAD: true, sendLength: false, sendRangeSize: true},
		},
	} {
		fsys := serve(t, tc.srv)

		info, err := fs.Stat(fsys, "de440s.bsp")
		if err != nil {
			t.Errorf("%s: Stat: %v", tc.name, err)

			continue
		}

		if info.Size() != int64(len(kernel)) {
			t.Errorf("%s: size %d, want %d", tc.name, info.Size(), len(kernel))
		}
	}
}

// TestHTTPMissingObjectIsErrNotExist is the error every cache-miss path in the
// module branches on.
//
// It is fs.ErrNotExist rather than an astrogo error on purpose: the same
// errors.Is works against os, embed, zip and every backend here, so a caller
// learns one vocabulary instead of two.
func TestHTTPMissingObjectIsErrNotExist(t *testing.T) {
	t.Parallel()

	fsys := serve(t, &objectServer{allowHEAD: true, sendLength: true, sendRangeSize: true})

	if _, err := fs.Stat(fsys, "nothing-here.bsp"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat of a missing object returned %v, want fs.ErrNotExist", err)
	}

	if _, err := fsys.Open("nothing-here.bsp"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Open of a missing object returned %v, want fs.ErrNotExist", err)
	}
}

// TestHTTPSequentialReadIsOneRequest pins the reason Read keeps a body open.
//
// A request per Read would be a request per buffer-full — for a 3 GB kernel at
// 32 KiB a read, about a hundred thousand of them. This asserts the design
// rather than the speed: one GET for a whole-object read.
func TestHTTPSequentialReadIsOneRequest(t *testing.T) {
	t.Parallel()

	srv := &objectServer{allowHEAD: true, sendLength: true, sendRangeSize: true}
	fsys := serve(t, srv)

	f, err := fsys.Open("de440s.bsp")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = f.Close() }()

	if _, err := io.ReadAll(f); err != nil {
		t.Fatal(err)
	}

	srv.mu.Lock()
	gets := srv.gets
	srv.mu.Unlock()

	if gets != 1 {
		t.Errorf("reading the object took %d GETs, want 1 — Read is not holding its body "+
			"open across calls", gets)
	}
}

// TestHTTPIsReadOnly records a deliberate absence.
//
// HTTP has PUT and DELETE. None of astrogo's sources accept them, and a backend
// that implemented CreateFS would be claiming a capability no endpoint in the
// registry has. The assertion is that the interfaces are NOT satisfied, so
// adding them later is a decision rather than an accident.
func TestHTTPIsReadOnly(t *testing.T) {
	t.Parallel()

	fsys := serve(t, &objectServer{allowHEAD: true, sendLength: true, sendRangeSize: true})

	if _, ok := fsys.(file.CreateFS); ok {
		t.Error("the HTTP backend implements CreateFS; no astrogo source accepts a write")
	}

	if _, ok := fsys.(file.RemoveFS); ok {
		t.Error("the HTTP backend implements RemoveFS")
	}

	if _, ok := fsys.(file.ContextFS); !ok {
		t.Error("the HTTP backend does not implement ContextFS, so a download over it " +
			"could not be cancelled")
	}
}

// TestHTTPHonoursItsContext is the property ContextFS exists for.
func TestHTTPHonoursItsContext(t *testing.T) {
	t.Parallel()

	fsys := serve(t, &objectServer{allowHEAD: true, sendLength: true, sendRangeSize: true})

	bound, err := file.RequireContext(t.Context(), fsys)
	if err != nil {
		t.Fatalf("RequireContext: %v", err)
	}

	if _, err := fs.Stat(bound, "de440s.bsp"); err != nil {
		t.Fatalf("a bound filesystem cannot read: %v", err)
	}

	// And a cancelled context stops it.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	cancelled, err := file.RequireContext(ctx, fsys)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := fs.Stat(cancelled, "de440s.bsp"); err == nil {
		t.Error("a cancelled context still fetched; ContextFS is not reaching the request")
	}
}

// TestMemFSSatisfiesTestFS holds the in-memory backend to the same bar.
func TestMemFSSatisfiesTestFS(t *testing.T) {
	t.Parallel()

	fsys, err := file.OpenFS("mem://conformance")
	if err != nil {
		t.Fatal(err)
	}

	cfs, ok := fsys.(file.CreateFS)
	if !ok {
		t.Fatal("mem:// does not implement CreateFS, which is the reason it exists rather " +
			"than fstest.MapFS")
	}

	names := []string{"kernels/de440s.bsp", "kernels/naif0012.tls", "eop/finals.data", "top.txt"}

	for _, name := range names {
		w, cerr := cfs.Create(name)
		if cerr != nil {
			t.Fatalf("Create %s: %v", name, cerr)
		}

		if _, werr := io.WriteString(w, "contents of "+name); werr != nil {
			t.Fatal(werr)
		}

		if cerr := w.Close(); cerr != nil {
			t.Fatal(cerr)
		}
	}

	if err := fstest.TestFS(fsys, names...); err != nil {
		t.Errorf("fstest.TestFS: %v", err)
	}
}

// TestMemFSHostsAreSeparateStores covers the addressing, which is how one test
// gives two components the same cache and another gives them different ones.
func TestMemFSHostsAreSeparateStores(t *testing.T) {
	t.Parallel()

	a, err := file.OpenFS("mem://shared-a")
	if err != nil {
		t.Fatal(err)
	}

	b, err := file.OpenFS("mem://shared-b")
	if err != nil {
		t.Fatal(err)
	}

	w, err := a.(file.CreateFS).Create("only-in-a.txt") //nolint:forcetypeassert // asserted above
	if err != nil {
		t.Fatal(err)
	}

	if _, err := io.WriteString(w, "x"); err != nil {
		t.Fatal(err)
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := fs.Stat(a, "only-in-a.txt"); err != nil {
		t.Errorf("the object is missing from its own host: %v", err)
	}

	if _, err := fs.Stat(b, "only-in-a.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a different mem:// host can see it (%v); the hosts are one store", err)
	}
}

// TestHTTPObjectMetadata pins what an HTTP object reports about itself.
//
// Each of these is a decision rather than an accident. Mode is 0444 and IsDir
// is always false because this filesystem serves objects and an HTTP server's
// directory listing is HTML, which io/fs cannot describe. Name is the base of
// the requested name, not the URL's last segment, so a ?key= wrapper serving
// one object under many names reports the name the caller asked for. ModTime
// comes from Last-Modified and is the zero time when the server does not say,
// which fs.FileInfo permits and which is honest.
func TestHTTPObjectMetadata(t *testing.T) {
	t.Parallel()

	const lastModified = "Wed, 21 Oct 2026 07:28:00 GMT"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Last-Modified", lastModified)
		w.Header().Set("Content-Length", "1234")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fsys, err := file.OpenFS(srv.URL + "/pub/naif/")
	if err != nil {
		t.Fatal(err)
	}

	info, err := fs.Stat(fsys, "kernels/de440s.bsp")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	if got := info.Name(); got != "de440s.bsp" {
		t.Errorf("Name() = %q, want the base of the requested name", got)
	}

	if got := info.Size(); got != 1234 {
		t.Errorf("Size() = %d, want 1234", got)
	}

	if got := info.Mode(); got != 0o444 {
		t.Errorf("Mode() = %v, want 0444 — an HTTP object is read-only", got)
	}

	if info.IsDir() {
		t.Error("IsDir() = true; this backend serves objects, never directories")
	}

	want, perr := http.ParseTime(lastModified)
	if perr != nil {
		t.Fatal(perr)
	}

	if !info.ModTime().Equal(want) {
		t.Errorf("ModTime() = %v, want the Last-Modified %v", info.ModTime(), want)
	}
}

// TestHTTPObjectWithoutALastModifiedReportsTheZeroTime covers the other branch,
// which matters because a zero ModTime is what makes [file.ETag] report "cannot
// tell" rather than synthesising a validator that would match itself for ever.
func TestHTTPObjectWithoutALastModifiedReportsTheZeroTime(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "7")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fsys, err := file.OpenFS(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}

	info, err := fs.Stat(fsys, "obj.dat")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	if !info.ModTime().IsZero() {
		t.Errorf("ModTime() = %v for a server that sent no Last-Modified, want the zero time",
			info.ModTime())
	}

	if got := file.ETag(info); got != "" {
		t.Errorf("ETag = %q with neither an ETag header nor a modification time, want \"\"", got)
	}
}
