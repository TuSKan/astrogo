package remote

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gofs "github.com/ungerik/go-fs"
)

// resumeBody is the full payload the fake server serves; the tests below
// pre-seed a partial containing its first N bytes.
const resumeBody = "0123456789abcdefghijABCDEFGHIJ"

// rangeServer serves resumeBody with real Range/If-Range semantics:
// a Range request whose If-Range matches etag gets a 206 with only the
// remaining bytes, anything else gets a plain 200 with the whole body.
// It records what it was asked for so a test can assert the client
// actually resumed rather than silently restarting.
type rangeServer struct {
	etag       string
	gotRange   string
	gotIfRange string
	servedFrom int64
	status     int
}

func (rs *rangeServer) start(t *testing.T) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rs.gotRange = r.Header.Get("Range")
		rs.gotIfRange = r.Header.Get("If-Range")

		w.Header().Set("ETag", rs.etag)

		var offset int64

		// Honor the range only when the validator still matches.
		if rs.gotRange != "" && rs.gotIfRange == rs.etag {
			if _, err := fmt.Sscanf(rs.gotRange, "bytes=%d-", &offset); err == nil && offset < int64(len(resumeBody)) {
				rs.servedFrom = offset
				rs.status = http.StatusPartialContent

				w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, len(resumeBody)-1, len(resumeBody)))
				w.WriteHeader(http.StatusPartialContent)

				_, _ = w.Write([]byte(resumeBody[offset:]))

				return
			}
		}

		rs.servedFrom = 0
		rs.status = http.StatusOK

		_, _ = w.Write([]byte(resumeBody))
	}))

	t.Cleanup(srv.Close)

	return srv
}

// seedPartial writes a partial body (and optionally its validator) into
// the cache location GetFile resolves for the fixture kernel, simulating
// a download that was interrupted partway.
func seedPartial(t *testing.T, content, validator string) {
	t.Helper()

	const name = "kernel.bsp"

	dir, err := CacheDir(NAIFSPK)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	if err := dir.MakeAllDirs(); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	dest := dir.Join(name)

	if err := partialFor(dest).WriteAll([]byte(content)); err != nil {
		t.Fatalf("seed partial: %v", err)
	}

	if validator != "" {
		if err := validatorFor(dest).WriteAll([]byte(validator)); err != nil {
			t.Fatalf("seed validator: %v", err)
		}
	}
}

// TestGetFileResumesFromPartial is the core of the feature: an interrupted
// download must continue from where it stopped, not re-fetch from zero.
func TestGetFileResumesFromPartial(t *testing.T) {
	cleanRemoteState(t)

	const done = 10

	rs := &rangeServer{etag: `"v1"`}
	srv := rs.start(t)

	if err := SetURL(NAIFSPK, srv.URL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	EnableDownloads(NAIFSPK, 0)
	seedPartial(t, resumeBody[:done], `"v1"`)

	f, err := GetFile(context.Background(), NAIFSPK, "kernel.bsp")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	if rs.status != http.StatusPartialContent {
		t.Errorf("server answered %d, want 206 — the client did not send a usable Range", rs.status)
	}

	if rs.gotRange != fmt.Sprintf("bytes=%d-", done) {
		t.Errorf("Range = %q, want bytes=%d-", rs.gotRange, done)
	}

	if rs.gotIfRange != `"v1"` {
		t.Errorf("If-Range = %q, want the stored ETag", rs.gotIfRange)
	}

	got, err := f.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	// The whole point: seeded prefix + resumed remainder == full body,
	// with no duplicated or dropped bytes at the seam.
	if string(got) != resumeBody {
		t.Errorf("resumed file = %q, want %q", got, resumeBody)
	}

	assertPartialCleared(t, f)
}

// TestGetFileRestartsWhenValidatorStale verifies If-Range's safety net: if
// upstream content changed, the server answers 200 with the whole body and
// the stale partial must be discarded, not appended to (which would splice
// old and new bytes into a corrupt file).
func TestGetFileRestartsWhenValidatorStale(t *testing.T) {
	cleanRemoteState(t)

	rs := &rangeServer{etag: `"v2"`} // server moved on
	srv := rs.start(t)

	if err := SetURL(NAIFSPK, srv.URL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	EnableDownloads(NAIFSPK, 0)
	seedPartial(t, "STALE-BYTES", `"v1"`) // validator no longer matches

	f, err := GetFile(context.Background(), NAIFSPK, "kernel.bsp")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	if rs.status != http.StatusOK {
		t.Errorf("server answered %d, want 200 for a stale validator", rs.status)
	}

	got, err := f.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if string(got) != resumeBody {
		t.Errorf("restarted file = %q, want exactly %q with no stale prefix", got, resumeBody)
	}

	if strings.Contains(string(got), "STALE") {
		t.Error("stale partial bytes survived into the final file")
	}

	assertPartialCleared(t, f)
}

// TestGetFileIgnoresPartialWithoutValidator verifies a partial with no
// recorded ETag is discarded rather than resumed — without a validator
// there is no way to prove it still matches upstream.
func TestGetFileIgnoresPartialWithoutValidator(t *testing.T) {
	cleanRemoteState(t)

	rs := &rangeServer{etag: `"v1"`}
	srv := rs.start(t)

	if err := SetURL(NAIFSPK, srv.URL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	EnableDownloads(NAIFSPK, 0)
	seedPartial(t, resumeBody[:10], "") // body, no validator

	f, err := GetFile(context.Background(), NAIFSPK, "kernel.bsp")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	if rs.gotRange != "" {
		t.Errorf("Range = %q, want none — a validator-less partial must not be resumed", rs.gotRange)
	}

	got, err := f.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if string(got) != resumeBody {
		t.Errorf("file = %q, want %q", got, resumeBody)
	}

	assertPartialCleared(t, f)
}

// TestGetFileResumeRespectsConsentSize verifies the consent gate is applied
// to the file's FULL size, not just the remaining leg — otherwise resuming
// would be a way to slip past a MaxDownloadSize cap a few bytes at a time.
func TestGetFileResumeRespectsConsentSize(t *testing.T) {
	cleanRemoteState(t)

	const done = 25 // only 5 bytes remain, but the file is 30

	rs := &rangeServer{etag: `"v1"`}
	srv := rs.start(t)

	if err := SetURL(NAIFSPK, srv.URL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	// Cap below the full size but above the remaining bytes.
	EnableDownloads(NAIFSPK, 10)
	seedPartial(t, resumeBody[:done], `"v1"`)

	_, err := GetFile(context.Background(), NAIFSPK, "kernel.bsp")
	if !errors.Is(err, ErrDownloadDenied) {
		t.Fatalf("expected ErrDownloadDenied (full size %d exceeds the 10-byte cap), got %v", len(resumeBody), err)
	}
}

// assertPartialCleared verifies a completed download leaves no sidecars.
func assertPartialCleared(t *testing.T, dest gofs.File) {
	t.Helper()

	if partialFor(dest).Exists() {
		t.Errorf("partial %s survived a completed download", partialFor(dest))
	}

	if validatorFor(dest).Exists() {
		t.Errorf("validator %s survived a completed download", validatorFor(dest))
	}
}
