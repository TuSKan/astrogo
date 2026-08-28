package remote

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"path"
	"strings"
	"sync"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote/file"
)

var errValidateTest = errors.New("not a valid LSK kernel")

func cleanRemoteState(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		SetDataDir("")
		Reset()
	})

	SetDataDir(testutil.FileURL(t, t.TempDir()))
}

// writeFakeSource is fakeSource (see resume_test.go) minus the return
// value — used by tests here that never need to inspect/mutate the
// source bucket directly after seeding it.
func writeFakeSource(t *testing.T, id EndpointID, name, content string) {
	t.Helper()

	fakeSource(t, id, name, content)
}

func TestGetFileImmutableExistenceOnly(t *testing.T) {
	cleanRemoteState(t)

	const payload = "kernel-v1"

	srcDir := t.TempDir()

	url := testutil.FileURL(t, srcDir)

	if err := SetURL(NAIFSPK, url); err != nil {
		t.Fatal(err)
	}

	srcBucket, err := file.Open(context.Background(), url)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := srcBucket.WriteAll(context.Background(), "planets/de440s.bsp", []byte(payload), nil); err != nil {
		t.Fatalf("seed source: %v", err)
	}

	EnableDownloads(0, NAIFSPK)

	bucket, key, err := GetFile(context.Background(), NAIFSPK, "planets/de440s.bsp")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	// Content correctness is asserted by the check below; the read itself
	// is not expected to fail here.
	data, _ := bucket.ReadAll(context.Background(), key)
	if string(data) != payload {
		t.Fatalf("unexpected content %q", data)
	}

	// Prove the second call reuses the cache without re-reading the
	// source at all (the real behavior an immutable endpoint's
	// !ep.Mutable fast path guarantees — freshInCache returns true on
	// existence alone) by deleting the source out from under it: a
	// re-fetch attempt would now fail outright, so a second success here
	// is only possible via the cache.
	if err := srcBucket.Delete(context.Background(), "planets/de440s.bsp"); err != nil {
		t.Fatalf("delete source: %v", err)
	}

	if _, _, err := GetFile(context.Background(), NAIFSPK, "planets/de440s.bsp"); err != nil {
		t.Fatalf("GetFile (cached, source now gone): %v", err)
	}
}

func TestGetFileMutableHeadProbeReuse(t *testing.T) {
	cleanRemoteState(t)

	const payload = "ngc-catalog-v1"

	writeFakeSource(t, OpenNGC, "NGC.csv", payload)

	EnableDownloads(0, OpenNGC)

	bucket, key, err := GetFile(context.Background(), OpenNGC, "NGC.csv")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	// Content correctness is asserted by the check below; the read itself
	// is not expected to fail here.
	data, _ := bucket.ReadAll(context.Background(), key)
	if string(data) != payload {
		t.Fatalf("unexpected content %q", data)
	}

	cachedAttrsBefore, err := bucket.Attributes(context.Background(), key)
	if err != nil {
		t.Fatalf("Attributes: %v", err)
	}

	// Second call: the source is untouched (same mtime/size, so the same
	// fileblob-derived ETag), so the cache must be reused untouched —
	// proved by the cache object's own ModTime staying identical, which
	// only happens if fetchInto/writeResumable's promote step never ran
	// a second time.
	if _, _, err := GetFile(context.Background(), OpenNGC, "NGC.csv"); err != nil {
		t.Fatalf("GetFile (reuse): %v", err)
	}

	cachedAttrsAfter, err := bucket.Attributes(context.Background(), key)
	if err != nil {
		t.Fatalf("Attributes (after): %v", err)
	}

	if !cachedAttrsAfter.ModTime.Equal(cachedAttrsBefore.ModTime) {
		t.Errorf("cache object was rewritten on an unchanged-source GetFile: ModTime %v -> %v", cachedAttrsBefore.ModTime, cachedAttrsAfter.ModTime)
	}
}

func TestGetFileMutableHeadProbeChanged(t *testing.T) {
	cleanRemoteState(t)

	srcBucket := fakeSource(t, OpenNGC, "NGC.csv", "ngc-catalog-v1")

	EnableDownloads(0, OpenNGC)

	if _, _, err := GetFile(context.Background(), OpenNGC, "NGC.csv"); err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	// The two versions differ in length, and that is load-bearing rather
	// than incidental.
	//
	// fileblob derives an ETag from the file's (ModTime, Size), and the
	// Windows system clock is coarse enough that two writes either side of a
	// local file copy can land on one tick. Both versions used to be
	// fourteen bytes, so when that happened the source's ETag was byte-for-
	// byte identical before and after the edit and unchanged correctly
	// reported no change — this test failed about one run in fifteen,
	// asserting something fileblob's own metadata cannot express.
	//
	// A differing length makes the change visible whichever branch of
	// unchanged is taken, so what is under test is the refresh behaviour
	// rather than the host's timer resolution. The same-length case is not
	// merely untested here but untestable through this backend; against the
	// real Mutable endpoints the ETag is server-supplied and content-based,
	// which is the case that matters.
	if err := srcBucket.WriteAll(context.Background(), "NGC.csv", []byte("ngc-catalog-version-2"), nil); err != nil {
		t.Fatalf("update source: %v", err)
	}

	bucket, key, err := GetFile(context.Background(), OpenNGC, "NGC.csv")
	if err != nil {
		t.Fatalf("GetFile (changed): %v", err)
	}

	// Content correctness is asserted by the check below; the read itself
	// is not expected to fail here.
	data, _ := bucket.ReadAll(context.Background(), key)
	if string(data) != "ngc-catalog-version-2" {
		t.Errorf("cache not refreshed after upstream change: got %q", data)
	}
}

func TestGetFileWithValidateRejectsCorruptDownload(t *testing.T) {
	cleanRemoteState(t)

	writeFakeSource(t, NAIFLSK, "naif0012.tls", "bad-lsk-content")

	EnableDownloads(0, NAIFLSK)

	_, _, err := GetFile(context.Background(), NAIFLSK, "naif0012.tls",
		WithValidate(func(io.Reader) error { return errValidateTest }))
	if !errors.Is(err, errValidateTest) {
		t.Fatalf("expected validate error, got %v", err)
	}

	// The CacheDir/Exists errors here are not the thing under test; a
	// failed existence check is not "exists" either way.
	bucket, prefix, _ := CacheDir(context.Background(), NAIFLSK)
	if exists, _ := bucket.Exists(context.Background(), prefix+"naif0012.tls"); exists {
		t.Error("a validate failure must not leave a cache file behind")
	}
}

func TestGetFileWithCacheNameDiffersFromPath(t *testing.T) {
	cleanRemoteState(t)

	// Real production shape of this option (see time/internal/iers's own
	// call): the fetched name and the on-disk cache name legitimately
	// differ. IERSFinals2000A's own real usage additionally passes name=""
	// (its URL alone names the whole HTTP resource) — untestable against a
	// local fake source, since a bucket always needs a real, non-empty key
	// unlike an HTTP URL that can BE the whole resource on its own; this
	// still exercises WithCacheName's actual differing-name behavior.
	writeFakeSource(t, OpenNGC, "raw-source-name.csv", "ngc-data")

	EnableDownloads(0, OpenNGC)

	_, key, err := GetFile(context.Background(), OpenNGC, "raw-source-name.csv", WithCacheName("NGC.csv"))
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	if path.Base(key) != "NGC.csv" {
		t.Errorf("cache key = %q, want base %q", key, "NGC.csv")
	}
}

func TestGetFileWithProgressReportsBytesDirectSavePath(t *testing.T) {
	cleanRemoteState(t)

	const payload = "kernel-progress-payload"

	writeFakeSource(t, NAIFSPK, "planets/progress.bsp", payload)

	EnableDownloads(0, NAIFSPK)

	var mu sync.Mutex

	var last int64

	var calls int

	_, _, err := GetFile(context.Background(), NAIFSPK, "planets/progress.bsp",
		WithProgress(func(downloaded, _ int64) {
			mu.Lock()
			defer mu.Unlock()

			calls++
			last = downloaded
		}))
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	if calls == 0 {
		t.Fatal("WithProgress callback was never invoked")
	}

	if last != int64(len(payload)) {
		t.Errorf("final downloaded = %d, want %d", last, len(payload))
	}

	calls = 0

	if _, _, err := GetFile(context.Background(), NAIFSPK, "planets/progress.bsp",
		WithProgress(func(int64, int64) { calls++ })); err != nil {
		t.Fatalf("GetFile (cached): %v", err)
	}

	if calls != 0 {
		t.Errorf("WithProgress must not fire on a cache hit; got %d calls", calls)
	}
}

func TestGetFileWithProgressReportsBytesValidatedPath(t *testing.T) {
	cleanRemoteState(t)

	const payload = "leap-second-progress-payload"

	writeFakeSource(t, NAIFLSK, "naif0012.tls", payload)

	EnableDownloads(0, NAIFLSK)

	var last int64

	_, _, err := GetFile(context.Background(), NAIFLSK, "naif0012.tls",
		WithValidate(func(io.Reader) error { return nil }),
		WithProgress(func(downloaded, _ int64) { last = downloaded }))
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	if last != int64(len(payload)) {
		t.Errorf("final downloaded = %d, want %d", last, len(payload))
	}
}

func TestGetFileDoesNotLogDownloadingWhenConsentDenied(t *testing.T) {
	cleanRemoteState(t)

	writeFakeSource(t, NAIFSPK, "planets/de442.bsp", "kernel-bytes")

	var logBuf bytes.Buffer

	prevOutput := log.Writer()
	prevFlags := log.Flags()

	log.SetOutput(&logBuf)
	log.SetFlags(0)

	t.Cleanup(func() {
		log.SetOutput(prevOutput)
		log.SetFlags(prevFlags)
	})

	_, _, err := GetFile(context.Background(), NAIFSPK, "planets/de442.bsp")
	if !errors.Is(err, ErrDownloadDenied) {
		t.Fatalf("expected ErrDownloadDenied, got %v", err)
	}

	if strings.Contains(logBuf.String(), "remote: downloading") {
		t.Errorf("logged a \"downloading\" message despite consent being denied:\n%s", logBuf.String())
	}
}

func TestGetFileDownloadDeniedWithoutConsent(t *testing.T) {
	cleanRemoteState(t)

	writeFakeSource(t, NAIFSPK, "planets/de442.bsp", "kernel-bytes")

	_, _, err := GetFile(context.Background(), NAIFSPK, "planets/de442.bsp")
	if !errors.Is(err, ErrDownloadDenied) {
		t.Fatalf("expected ErrDownloadDenied, got %v", err)
	}

	// The CacheDir/Exists errors here are not the thing under test; a
	// failed existence check is not "exists" either way.
	bucket, prefix, _ := CacheDir(context.Background(), NAIFSPK)
	if exists, _ := bucket.Exists(context.Background(), prefix+"de442.bsp"); exists {
		t.Error("denied download must not create a cache file")
	}
}

func TestGetFileRespectsOfflineAndDisable(t *testing.T) {
	cleanRemoteState(t)

	EnableDownloads(0, NAIFSPK)

	Disable(NAIFSPK)

	if _, _, err := GetFile(context.Background(), NAIFSPK, "planets/de442.bsp"); !errors.Is(err, ErrEndpointDisabled) {
		t.Errorf("expected ErrEndpointDisabled, got %v", err)
	}

	Enable(NAIFSPK)
	SetOffline(true)

	if _, _, err := GetFile(context.Background(), NAIFSPK, "planets/de442.bsp"); !errors.Is(err, ErrOffline) {
		t.Errorf("expected ErrOffline, got %v", err)
	}
}

func TestGetFileWithDownloadTimeoutOverridesEndpointDefault(t *testing.T) {
	cleanRemoteState(t)

	writeFakeSource(t, NAIFSPK, "planets/slow.bsp", "too-slow")

	EnableDownloads(0, NAIFSPK)

	// A local file read is effectively instantaneous, so there is no way to
	// simulate real transport latency the way an artificially slow httptest
	// handler used to. An already-past deadline exercises the same
	// timeout-propagation path without depending on wall-clock delay.
	//
	// Past rather than merely tiny: this used to pass 1ns, which is a race
	// and not a guarantee. context.WithTimeout starts a timer, so with a
	// positive duration the context is only done once that timer has fired,
	// and a local Attributes call can win — the test failed about one run in
	// fifteen. A non-positive duration is different in kind, not degree:
	// context.WithDeadline cancels immediately when the deadline has already
	// passed, so the context handed to srcBucket.Attributes/NewRangeReader
	// inside fetchInto is done before either call is made.
	//
	// Note this reaches fetchInto at all only because a negative duration
	// survives the cmp.Or in GetFile, which treats zero — and only zero — as
	// "unset, use the endpoint default".
	_, _, err := GetFile(context.Background(), NAIFSPK, "planets/slow.bsp", WithDownloadTimeout(-1))
	if err == nil {
		t.Fatal("expected the request to time out")
	}
}

func TestGetFileReturnsUsableBucketForAllReadModes(t *testing.T) {
	cleanRemoteState(t)

	const payload = "random-access-content"

	writeFakeSource(t, NAIFSPK, "planets/de440s.bsp", payload)

	EnableDownloads(0, NAIFSPK)

	bucket, key, err := GetFile(context.Background(), NAIFSPK, "planets/de440s.bsp")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	if all, err := bucket.ReadAll(context.Background(), key); err != nil || string(all) != payload {
		t.Errorf("ReadAll = %q, %v; want %q, nil", all, err, payload)
	}

	r, err := bucket.NewReader(context.Background(), key, nil)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}

	// Content correctness is asserted by the check below.
	seqData, _ := io.ReadAll(r)
	_ = r.Close()

	if string(seqData) != payload {
		t.Errorf("NewReader content = %q, want %q", seqData, payload)
	}

	rr, err := bucket.NewRangeReader(context.Background(), key, 0, int64(len(payload)), nil)
	if err != nil {
		t.Fatalf("NewRangeReader: %v", err)
	}
	defer rr.Close() //nolint:errcheck // test

	buf := make([]byte, len(payload))
	if _, err := io.ReadFull(rr, buf); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}

	if string(buf) != payload {
		t.Errorf("NewRangeReader content = %q, want %q", buf, payload)
	}
}
