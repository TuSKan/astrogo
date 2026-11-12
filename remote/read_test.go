package remote

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/remote/file"
	"github.com/TuSKan/astrogo/time"
)

// What these tests serve and cache. A read accepts goodCopy and nothing else;
// the two versions differ in length so that no weak validator can confuse them.
const (
	readName   = "catalog.dat"
	goodCopy   = "a catalog its reader accepts"
	badCopy    = "<html>503</html>"
	damagedOld = "a catalog damaged where it sat"
)

var errUnreadable = errors.New("not a catalog")

// versionedSource is an HTTP source serving one object whose content and ETag a
// test can change, counting the requests that fetch its body.
type versionedSource struct {
	mu     sync.Mutex
	body   string
	etag   string // "" sends no ETag, and no Last-Modified either
	bodies atomic.Int32
}

func (v *versionedSource) serve(body, etag string) {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.body, v.etag = body, etag
}

// downloads is how many times the body was fetched. The backend's ranged
// one-byte probe is metadata, like its HEAD, and is not counted.
func (v *versionedSource) downloads() int32 { return v.bodies.Load() }

// serveVersioned points id at a versionedSource serving body under etag, and
// consents to id's downloads.
func serveVersioned(t *testing.T, id EndpointID, body, etag string) *versionedSource {
	t.Helper()

	v := &versionedSource{}
	v.serve(body, etag)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimPrefix(r.URL.Path, "/") != readName {
			http.NotFound(w, r)

			return
		}

		v.mu.Lock()
		body, etag := v.body, v.etag
		v.mu.Unlock()

		if etag != "" {
			w.Header().Set("ETag", `"`+etag+`"`)
		}

		if r.Method == http.MethodGet && r.Header.Get("Range") != "bytes=0-0" {
			v.bodies.Add(1)
		}

		// A zero modification time sends no Last-Modified, so an etag of ""
		// leaves the object with no validator at all.
		http.ServeContent(w, r, readName, time.GoTime{}, strings.NewReader(body))
	}))
	t.Cleanup(srv.Close)

	if err := SetURL(id, srv.URL+"/"); err != nil {
		t.Fatal(err)
	}

	EnableDownloads(0, id)

	return v
}

// reading is a WithRead read that accepts goodCopy alone, recording every
// copy it was handed.
type reading struct {
	mu   sync.Mutex
	seen []string
}

func (r *reading) read(fsys FS, key string) error {
	b, err := fs.ReadFile(fsys, key)
	if err != nil {
		return fmt.Errorf("read %s: %w", key, err)
	}

	r.mu.Lock()
	r.seen = append(r.seen, string(b))
	r.mu.Unlock()

	if string(b) != goodCopy {
		return errUnreadable
	}

	return nil
}

func (r *reading) copies() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]string(nil), r.seen...)
}

// cacheKeyFor is where id caches readName.
func cacheKeyFor(t *testing.T, id EndpointID) (FS, string) {
	t.Helper()

	fsys, prefix, err := CacheDir(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}

	return fsys, prefix + readName
}

func isCached(t *testing.T, fsys FS, key string) bool {
	t.Helper()

	_, err := fs.Stat(fsys, key)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Stat %s: %v", key, err)
	}

	return err == nil
}

// Both kinds of endpoint, since they reach a cached copy by different paths:
// an immutable one on existence alone, a Mutable one after its recorded source
// ETag matches the source's.
var readEndpoints = []struct {
	name string
	id   EndpointID
}{
	{"immutable", NAIFSPK},
	{"mutable", OpenNGC},
}

// seedDamaged caches damagedOld for id as if GetFile had fetched it at
// source version etag, so GetFile serves it without asking for the body.
func seedDamaged(t *testing.T, id EndpointID, etag string) {
	t.Helper()

	ctx := context.Background()
	fsys, key := cacheKeyFor(t, id)

	if err := WriteFile(ctx, fsys, key, strings.NewReader(damagedOld)); err != nil {
		t.Fatal(err)
	}

	if err := file.WriteETag(ctx, fsys, key, etag); err != nil {
		t.Fatal(err)
	}
}

// A cached copy its reader refuses may have been damaged where it sat, while
// the source still serves a good one. It is replaced in the same call: the
// caller sees the good copy and no error.
func TestWithReadReplacesADamagedCachedCopy(t *testing.T) {
	for _, ep := range readEndpoints {
		t.Run(ep.name, func(t *testing.T) {
			cleanRemoteState(t)

			src := serveVersioned(t, ep.id, goodCopy, "v1")
			seedDamaged(t, ep.id, "v1")

			var r reading

			fsys, key, err := GetFile(context.Background(), ep.id, readName, WithRead(r.read))
			if err != nil {
				t.Fatalf("GetFile over a damaged copy: %v", err)
			}

			if got, want := r.copies(), []string{damagedOld, goodCopy}; !slices.Equal(got, want) {
				t.Errorf("read was handed %q, want %q", got, want)
			}

			if b, _ := fs.ReadFile(fsys, key); string(b) != goodCopy {
				t.Errorf("cache holds %q, want %q", b, goodCopy)
			}

			if n := src.downloads(); n != 1 {
				t.Errorf("%d downloads, want 1", n)
			}
		})
	}
}

// A copy refused right after its download is what the source serves. GetFile
// downloads it once, then refuses to download it again while the source still
// serves that version, and fetches again once the source changes.
func TestWithReadDownloadsARefusedVersionOnce(t *testing.T) {
	for _, ep := range readEndpoints {
		t.Run(ep.name, func(t *testing.T) {
			cleanRemoteState(t)

			src := serveVersioned(t, ep.id, badCopy, "v1")
			cache, key := cacheKeyFor(t, ep.id)
			ctx := context.Background()

			var r reading

			if _, _, err := GetFile(ctx, ep.id, readName, WithRead(r.read)); !errors.Is(err, errUnreadable) {
				t.Fatalf("first GetFile: err %v, want the read's own error", err)
			}

			if isCached(t, cache, key) {
				t.Error("the refused copy is still cached")
			}

			if isCached(t, cache, key+file.SourceETagSuffix) {
				t.Error("the refused copy's recorded source ETag is still cached")
			}

			for call := 2; call <= 3; call++ {
				_, _, err := GetFile(ctx, ep.id, readName, WithRead(r.read))
				if !errors.Is(err, ErrRejectedSource) {
					t.Fatalf("call %d, source unchanged: err %v, want ErrRejectedSource", call, err)
				}

				if !strings.Contains(err.Error(), errUnreadable.Error()) {
					t.Errorf("call %d: %q does not say why the copy was refused", call, err)
				}
			}

			if n := src.downloads(); n != 1 {
				t.Errorf("%d downloads over three calls, want 1", n)
			}

			if n := len(r.copies()); n != 1 {
				t.Errorf("read was called %d times, want 1", n)
			}

			src.serve(goodCopy, "v2")

			if _, _, err := GetFile(ctx, ep.id, readName, WithRead(r.read)); err != nil {
				t.Fatalf("GetFile after the source changed: %v", err)
			}

			if n := src.downloads(); n != 2 {
				t.Errorf("%d downloads after the source changed, want 2", n)
			}
		})
	}
}

// A damaged cached copy over a source that serves a bad one too: the heal
// downloads once, finds the source to blame, and later calls download nothing.
func TestWithReadDamagedCopyOverABadSource(t *testing.T) {
	cleanRemoteState(t)

	src := serveVersioned(t, NAIFSPK, badCopy, "v1")
	seedDamaged(t, NAIFSPK, "v0")

	var r reading

	if _, _, err := GetFile(context.Background(), NAIFSPK, readName, WithRead(r.read)); !errors.Is(err, errUnreadable) {
		t.Fatalf("first GetFile: err %v, want the read's own error", err)
	}

	if got, want := r.copies(), []string{damagedOld, badCopy}; !slices.Equal(got, want) {
		t.Errorf("read was handed %q, want %q", got, want)
	}

	if _, _, err := GetFile(context.Background(), NAIFSPK, readName, WithRead(r.read)); !errors.Is(err, ErrRejectedSource) {
		t.Fatalf("second GetFile: err %v, want ErrRejectedSource", err)
	}

	if n := src.downloads(); n != 1 {
		t.Errorf("%d downloads, want 1", n)
	}
}

// A heal that cannot fetch says so, with the read's error, and leaves nothing
// cached: here the caller may not download.
func TestWithReadHealWithoutConsent(t *testing.T) {
	cleanRemoteState(t)

	serveVersioned(t, NAIFSPK, goodCopy, "v1")
	seedDamaged(t, NAIFSPK, "v1")
	DisableDownloads(NAIFSPK)

	var r reading

	_, _, err := GetFile(context.Background(), NAIFSPK, readName, WithRead(r.read))
	if !errors.Is(err, errUnreadable) || !errors.Is(err, ErrDownloadDenied) {
		t.Fatalf("err %v, want both the read's error and ErrDownloadDenied", err)
	}

	if cache, key := cacheKeyFor(t, NAIFSPK); isCached(t, cache, key) {
		t.Error("the refused copy is still cached")
	}
}

// A read that fails because its context ended has learned nothing about the
// copy, which stays cached, and nothing is fetched.
func TestWithReadKeepsTheCopyWhenTheContextEnds(t *testing.T) {
	cleanRemoteState(t)

	src := serveVersioned(t, NAIFSPK, goodCopy, "v1")
	seedDamaged(t, NAIFSPK, "v1")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	read := func(FS, string) error {
		cancel()

		return ctx.Err()
	}

	if _, _, err := GetFile(ctx, NAIFSPK, readName, WithRead(read)); !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v, want context.Canceled", err)
	}

	if cache, key := cacheKeyFor(t, NAIFSPK); !isCached(t, cache, key) {
		t.Error("the copy was removed though the read only stopped")
	}

	if n := src.downloads(); n != 0 {
		t.Errorf("%d downloads, want 0", n)
	}
}

// Nor does it reject the version it was reading. Here the copy was just
// downloaded, so a rejection would refuse the next fetch of it: once the copy
// is gone, the next call must download it and succeed.
func TestWithReadDoesNotRejectAVersionWhenTheContextEnds(t *testing.T) {
	cleanRemoteState(t)

	src := serveVersioned(t, NAIFSPK, goodCopy, "v1")
	cache, key := cacheKeyFor(t, NAIFSPK)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := func(FS, string) error {
		cancel()

		return ctx.Err()
	}

	if _, _, err := GetFile(ctx, NAIFSPK, readName, WithRead(stop)); !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v, want context.Canceled", err)
	}

	if err := RemoveFile(context.Background(), cache, key); err != nil {
		t.Fatalf("remove the copy the stopped read left: %v", err)
	}

	var r reading

	if _, _, err := GetFile(context.Background(), NAIFSPK, readName, WithRead(r.read)); err != nil {
		t.Fatalf("GetFile after a stopped read: %v", err)
	}

	if n := src.downloads(); n != 2 {
		t.Errorf("%d downloads, want 2", n)
	}
}

// A validator's refusal rejects the version the same way, so WithValidate
// alone stops the same bytes being downloaded on every call.
func TestWithValidateDownloadsARefusedVersionOnce(t *testing.T) {
	cleanRemoteState(t)

	src := serveVersioned(t, OpenNGC, badCopy, "v1")
	validate := WithValidate(func(io.Reader) error { return errUnreadable })

	if _, _, err := GetFile(context.Background(), OpenNGC, readName, validate); !errors.Is(err, errUnreadable) {
		t.Fatalf("first GetFile: err %v, want the validator's error", err)
	}

	if _, _, err := GetFile(context.Background(), OpenNGC, readName, validate); !errors.Is(err, ErrRejectedSource) {
		t.Fatalf("second GetFile: err %v, want ErrRejectedSource", err)
	}

	if n := src.downloads(); n != 1 {
		t.Errorf("%d downloads, want 1", n)
	}
}

// A rejection lives in the client that made it and is not written to the
// cache. Another client sharing the cache fetches again, as a new process
// would, so a newer reader is never refused a file an older one rejected.
func TestARejectionIsTheClientsOwn(t *testing.T) {
	cleanRemoteState(t)

	src := serveVersioned(t, NAIFSPK, badCopy, "v1")

	var r reading

	if _, _, err := GetFile(context.Background(), NAIFSPK, readName, WithRead(r.read)); !errors.Is(err, errUnreadable) {
		t.Fatalf("first GetFile: err %v, want the read's own error", err)
	}

	cache, prefix, err := CacheDir(context.Background(), NAIFSPK)
	if err != nil {
		t.Fatal(err)
	}

	if entries, err := fs.ReadDir(cache, strings.TrimSuffix(prefix, "/")); err == nil && len(entries) > 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}

		t.Errorf("the cache holds %q after a rejection, want nothing", names)
	}

	other := Default().Clone()

	if _, _, err := other.GetFile(context.Background(), NAIFSPK, readName, WithRead(r.read)); !errors.Is(err, errUnreadable) {
		t.Fatalf("another client: err %v, want the read's own error after a fresh download", err)
	}

	if n := src.downloads(); n != 2 {
		t.Errorf("%d downloads, want 2: one per client", n)
	}
}

// A source that reports no validator cannot be recognized again, so a refused
// copy from it is downloaded again on every call, as WithRead documents.
func TestWithReadSourceWithoutAValidatorIsFetchedEachTime(t *testing.T) {
	cleanRemoteState(t)

	src := serveVersioned(t, NAIFSPK, badCopy, "")

	var r reading

	for call := 1; call <= 2; call++ {
		if _, _, err := GetFile(context.Background(), NAIFSPK, readName, WithRead(r.read)); !errors.Is(err, errUnreadable) {
			t.Fatalf("call %d: err %v, want the read's own error", call, err)
		}
	}

	if n := src.downloads(); n != 2 {
		t.Errorf("%d downloads, want 2", n)
	}
}

// A rejection is keyed by data directory as well as cache key, so pointing
// the client at a different cache does not carry it over.
func TestARejectionStaysWithItsCache(t *testing.T) {
	cleanRemoteState(t)

	src := serveVersioned(t, NAIFSPK, badCopy, "v1")

	var r reading

	if _, _, err := GetFile(context.Background(), NAIFSPK, readName, WithRead(r.read)); !errors.Is(err, errUnreadable) {
		t.Fatalf("first GetFile: err %v, want the read's own error", err)
	}

	cleanRemoteState(t)
	serveVersioned(t, NAIFSPK, badCopy, "v1")

	if _, _, err := GetFile(context.Background(), NAIFSPK, readName, WithRead(r.read)); !errors.Is(err, errUnreadable) {
		t.Fatalf("GetFile into a new cache: err %v, want the read's own error after a fresh download", err)
	}

	if n := src.downloads(); n != 1 {
		t.Errorf("the first source served %d downloads, want 1", n)
	}
}

// A source that starts serving something the reader refuses does not cost the
// cache the copy that worked. The download was promoted over the cached copy
// and read after, so its refusal removed the only copy there was, and every
// later call failed with nothing cached, offline ones included (#1047).
func TestWithReadKeepsTheCachedCopyOverARefusedUpdate(t *testing.T) {
	cleanRemoteState(t)

	ctx := context.Background()
	src := serveVersioned(t, OpenNGC, goodCopy, "v1")

	var r reading

	if _, _, err := GetFile(ctx, OpenNGC, readName, WithRead(r.read)); err != nil {
		t.Fatalf("first GetFile: %v", err)
	}

	src.serve(badCopy, "v2")

	if _, _, err := GetFile(ctx, OpenNGC, readName, WithRead(r.read)); !errors.Is(err, errUnreadable) {
		t.Fatalf("GetFile of the refused update: err %v, want the read's own error", err)
	}

	fsys, key := cacheKeyFor(t, OpenNGC)

	if b, err := fs.ReadFile(fsys, key); err != nil || string(b) != goodCopy {
		t.Fatalf("cache holds %q (err %v), want the good copy kept", b, err)
	}

	if etag := file.RecordedETag(ctx, fsys, key); etag != "v1" {
		t.Errorf("recorded ETag %q, want the kept copy's v1", etag)
	}

	// The version is still rejected, without a second download of it.
	if _, _, err := GetFile(ctx, OpenNGC, readName, WithRead(r.read)); !errors.Is(err, ErrRejectedSource) {
		t.Errorf("GetFile with the source unchanged: err %v, want ErrRejectedSource", err)
	}

	if n := src.downloads(); n != 2 {
		t.Errorf("%d downloads, want 2: the good version and the refused one, once", n)
	}

	// And an offline caller is served the copy that worked.
	SetOffline(true)
	t.Cleanup(func() { SetOffline(false) })

	if _, _, err := GetFile(ctx, OpenNGC, readName, WithRead(r.read)); err != nil {
		t.Errorf("offline GetFile after the refused update: %v", err)
	}
}

// An update the reader accepts replaces the cached copy as before. It is read
// twice: once staged, to decide whether it may replace the copy, and once where
// GetFile returns it, which is the read a caller keeps.
func TestWithReadPromotesAnUpdateItAccepts(t *testing.T) {
	cleanRemoteState(t)

	const newer = "a newer catalog its reader also accepts"

	ctx := context.Background()
	src := serveVersioned(t, OpenNGC, goodCopy, "v1")

	var seen []string

	read := func(fsys FS, key string) error {
		b, err := fs.ReadFile(fsys, key)
		if err != nil {
			return fmt.Errorf("read %s: %w", key, err)
		}

		seen = append(seen, string(b))

		return nil
	}

	if _, _, err := GetFile(ctx, OpenNGC, readName, WithRead(read)); err != nil {
		t.Fatalf("first GetFile: %v", err)
	}

	src.serve(newer, "v2")

	fsys, key, err := GetFile(ctx, OpenNGC, readName, WithRead(read))
	if err != nil {
		t.Fatalf("GetFile of the update: %v", err)
	}

	if b, _ := fs.ReadFile(fsys, key); string(b) != newer {
		t.Errorf("cache holds %q, want the update", b)
	}

	if want := []string{goodCopy, newer, newer}; !slices.Equal(seen, want) {
		t.Errorf("read was handed %q, want %q", seen, want)
	}
}
