package remote

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// failingSource points id at a server that answers every request with 503,
// counting them, and returns the count.
func failingSource(t *testing.T, id EndpointID) *atomic.Int32 {
	t.Helper()

	var requests atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	if err := SetURL(id, srv.URL+"/"); err != nil {
		t.Fatal(err)
	}

	return &requests
}

// silentSource points id at a server that accepts every request and never
// answers it, counting them, and returns the count.
func silentSource(t *testing.T, id EndpointID) *atomic.Int32 {
	t.Helper()

	var requests atomic.Int32

	release := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		requests.Add(1)

		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	if err := SetURL(id, srv.URL+"/"); err != nil {
		t.Fatal(err)
	}

	return &requests
}

// seedCached writes goodCopy where id caches readName, as a copy pre-seeded
// by hand or left by an earlier run.
func seedCached(t *testing.T, id EndpointID) {
	t.Helper()

	fsys, key := cacheKeyFor(t, id)
	if err := WriteFile(context.Background(), fsys, key, strings.NewReader(goodCopy)); err != nil {
		t.Fatalf("seed the cache: %v", err)
	}
}

// readCached is the content GetFile handed back.
func readCached(t *testing.T, fsys FS, key string) string {
	t.Helper()

	b, err := fs.ReadFile(fsys, key)
	if err != nil {
		t.Fatalf("read %s: %v", key, err)
	}

	return string(b)
}

// TestGetFileServesACachedCopyItsSourceCannotConfirm is #1279: a Mutable
// endpoint's cached copy, with its source answering every request with an
// error, is served after one request, as offline mode would serve it. GetFile
// used to refuse the copy, after asking the source four times.
func TestGetFileServesACachedCopyItsSourceCannotConfirm(t *testing.T) {
	cleanRemoteState(t)

	seedCached(t, OpenNGC)
	requests := failingSource(t, OpenNGC)

	fsys, key, err := GetFile(context.Background(), OpenNGC, readName)
	if err != nil {
		t.Fatalf("GetFile with the source failing and a copy cached: %v", err)
	}

	if got := readCached(t, fsys, key); got != goodCopy {
		t.Errorf("GetFile served %q, want the cached copy", got)
	}

	if n := requests.Load(); n != 1 {
		t.Errorf("the source was asked %d times, want once", n)
	}
}

// TestGetFileBoundsTheFreshnessCheck: a source that never answers is asked
// under the endpoint's download timeout, here shortened, and its cached copy is
// served when that runs out. Only the download used to be bounded, and the
// freshness checks around it waited for as long as the connection did.
func TestGetFileBoundsTheFreshnessCheck(t *testing.T) {
	cleanRemoteState(t)

	seedCached(t, OpenNGC)
	silentSource(t, OpenNGC)

	start := time.Now()

	fsys, key, err := GetFile(context.Background(), OpenNGC, readName, WithDownloadTimeout(200*time.Millisecond))
	if err != nil {
		t.Fatalf("GetFile with the source silent and a copy cached: %v", err)
	}

	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("GetFile took %v against a 200 ms timeout", took)
	}

	if got := readCached(t, fsys, key); got != goodCopy {
		t.Errorf("GetFile served %q, want the cached copy", got)
	}
}

// TestGetFileAsksASilentHostOnce: a source host that gave no answer is not
// asked about a second file inside the client's cooldown, as it would not be
// for an API call. OpenNGC's two CSVs, cached, waited out one connect timeout
// each behind a firewall that dropped packets.
func TestGetFileAsksASilentHostOnce(t *testing.T) {
	cleanRemoteState(t)

	seedCached(t, OpenNGC)

	fsys, prefix, err := CacheDir(context.Background(), OpenNGC)
	if err != nil {
		t.Fatal(err)
	}

	const other = "other.dat"
	if err := WriteFile(context.Background(), fsys, prefix+other, strings.NewReader(goodCopy)); err != nil {
		t.Fatalf("seed %s: %v", other, err)
	}

	requests := silentSource(t, OpenNGC)

	for _, name := range []string{readName, other} {
		if _, _, err := GetFile(context.Background(), OpenNGC, name, WithDownloadTimeout(200*time.Millisecond)); err != nil {
			t.Fatalf("GetFile(%s) with the host silent and a copy cached: %v", name, err)
		}
	}

	if n := requests.Load(); n != 1 {
		t.Errorf("the silent host was asked %d times for two files, want once", n)
	}
}

// TestGetFileDoesNotRememberALocalSourceAsSilent: a file:// source that lacks
// the object has answered, and is asked again as soon as the object is there.
// Telling a transport failure apart by net.Error took this for one, since a
// missing file's syscall.Errno satisfies it, and refused the second call with
// "no answer".
func TestGetFileDoesNotRememberALocalSourceAsSilent(t *testing.T) {
	cleanRemoteState(t)

	src := fakeSource(t, OpenNGC, "unrelated.dat", "x")

	if _, _, err := GetFile(context.Background(), OpenNGC, readName); err == nil {
		t.Fatal("GetFile of an object the source lacks returned no error")
	}

	if err := WriteFile(context.Background(), src, readName, strings.NewReader(goodCopy)); err != nil {
		t.Fatalf("add the object to the source: %v", err)
	}

	fsys, key, err := GetFile(context.Background(), OpenNGC, readName)
	if err != nil {
		t.Fatalf("GetFile once the source has the object: %v", err)
	}

	if got := readCached(t, fsys, key); got != goodCopy {
		t.Errorf("GetFile served %q, want the source's copy", got)
	}
}

// TestWithoutCacheFallbackServesOnlyAConfirmedCopy: with WithoutCacheFallback
// a Mutable endpoint's cached copy is served only when its source confirms it,
// and each way of not asking fails with its own reason. An immutable copy is
// current by definition and is served regardless.
func TestWithoutCacheFallbackServesOnlyAConfirmedCopy(t *testing.T) {
	t.Run("the source errs", func(t *testing.T) {
		cleanRemoteState(t)
		seedCached(t, OpenNGC)
		requests := failingSource(t, OpenNGC)

		if _, _, err := GetFile(context.Background(), OpenNGC, readName, WithoutCacheFallback()); !errors.Is(err, ErrDownloadFailed) {
			t.Errorf("err = %v, want ErrDownloadFailed", err)
		}

		if n := requests.Load(); n != 1 {
			t.Errorf("the source was asked %d times, want once", n)
		}
	})

	t.Run("offline", func(t *testing.T) {
		cleanRemoteState(t)
		seedCached(t, OpenNGC)

		if err := SetURL(OpenNGC, offlineHost); err != nil {
			t.Fatal(err)
		}

		SetOffline(true)

		if _, _, err := GetFile(context.Background(), OpenNGC, readName, WithoutCacheFallback()); !errors.Is(err, ErrOffline) {
			t.Errorf("err = %v, want ErrOffline", err)
		}
	})

	t.Run("a refusing policy", func(t *testing.T) {
		cleanRemoteState(t)
		seedCached(t, OpenNGC)
		failingSource(t, OpenNGC)

		SetPolicy(func(Endpoint, int64) error { return errRefusedHere })

		if _, _, err := GetFile(context.Background(), OpenNGC, readName, WithoutCacheFallback()); !errors.Is(err, ErrDownloadDenied) {
			t.Errorf("err = %v, want ErrDownloadDenied", err)
		}
	})

	t.Run("a host that gave no answer", func(t *testing.T) {
		cleanRemoteState(t)
		seedCached(t, OpenNGC)
		requests := silentSource(t, OpenNGC)

		if _, _, err := GetFile(context.Background(), OpenNGC, readName, WithDownloadTimeout(200*time.Millisecond)); err != nil {
			t.Fatalf("GetFile with the host silent and a copy cached: %v", err)
		}

		_, _, err := GetFile(context.Background(), OpenNGC, readName, WithoutCacheFallback())
		if !errors.Is(err, ErrDownloadFailed) || !errors.Is(err, ErrUnreachable) {
			t.Errorf("err = %v, want ErrDownloadFailed and ErrUnreachable", err)
		}

		if n := requests.Load(); n != 1 {
			t.Errorf("the silent host was asked %d times, want only by the first call", n)
		}
	})

	t.Run("a copy the source confirms", func(t *testing.T) {
		cleanRemoteState(t)
		src := serveVersioned(t, OpenNGC, goodCopy, "v1")

		if _, _, err := GetFile(context.Background(), OpenNGC, readName); err != nil {
			t.Fatalf("first GetFile: %v", err)
		}

		fsys, key, err := GetFile(context.Background(), OpenNGC, readName, WithoutCacheFallback())
		if err != nil {
			t.Fatalf("GetFile of a confirmed copy: %v", err)
		}

		if got := readCached(t, fsys, key); got != goodCopy {
			t.Errorf("GetFile served %q, want the confirmed copy", got)
		}

		if n := src.downloads(); n != 1 {
			t.Errorf("%d downloads, want only the first", n)
		}
	})

	t.Run("an immutable copy", func(t *testing.T) {
		cleanRemoteState(t)
		seedCached(t, NAIFSPK)

		if err := SetURL(NAIFSPK, offlineHost); err != nil {
			t.Fatal(err)
		}

		SetOffline(true)

		if _, _, err := GetFile(context.Background(), NAIFSPK, readName, WithoutCacheFallback()); err != nil {
			t.Errorf("GetFile of a cached immutable copy, offline: %v", err)
		}
	})
}

// TestGetFileTakesAMissingSourceObjectAsAnAnswer: a source that no longer has
// a Mutable endpoint's object has answered, and the cached copy is not
// current. GetFile fetches it and fails as the source does, where a source
// that could not be asked has its copy served.
func TestGetFileTakesAMissingSourceObjectAsAnAnswer(t *testing.T) {
	cleanRemoteState(t)
	seedCached(t, OpenNGC)

	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)

	if err := SetURL(OpenNGC, srv.URL+"/"); err != nil {
		t.Fatal(err)
	}

	if _, _, err := GetFile(context.Background(), OpenNGC, readName); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want fs.ErrNotExist", err)
	}
}

// TestGetFileComparesSizesWithoutARecordedETag: a Mutable copy with no source
// ETag recorded beside it, as one seeded by hand has none, is compared with its
// source by size. The same size is current and is served with no download;
// another size is downloaded.
func TestGetFileComparesSizesWithoutARecordedETag(t *testing.T) {
	const longer = "a newer catalog, longer than the cached one"

	for _, tc := range []struct {
		name, served, want string
		downloads          int32
	}{
		{"the same size", strings.Repeat("x", len(goodCopy)), goodCopy, 0},
		{"another size", longer, longer, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cleanRemoteState(t)
			seedCached(t, OpenNGC)
			src := serveVersioned(t, OpenNGC, tc.served, "")

			fsys, key, err := GetFile(context.Background(), OpenNGC, readName)
			if err != nil {
				t.Fatalf("GetFile: %v", err)
			}

			if got := readCached(t, fsys, key); got != tc.want {
				t.Errorf("GetFile served %q, want %q", got, tc.want)
			}

			if n := src.downloads(); n != tc.downloads {
				t.Errorf("%d downloads, want %d", n, tc.downloads)
			}
		})
	}
}

// errRefusedHere is the reason this file's refusing policy gives.
var errRefusedHere = errors.New("refused by this test's policy")
