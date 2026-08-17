package spk

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/remote/api"
	"github.com/TuSKan/astrogo/remote/file"
	"github.com/TuSKan/astrogo/time"
)

// testDAFHeader builds a minimal but structurally valid DAF/SPK file
// record: FWD/BWD zero (no summary records), FREE=1 (trivial min-size
// check), little-endian marker.
func testDAFHeader() []byte {
	buf := make([]byte, RecordSize)
	order := binary.LittleEndian

	copy(buf[0:8], "NAIF/DAF")
	order.PutUint32(buf[8:12], 2)  // ND
	order.PutUint32(buf[12:16], 6) // NI
	order.PutUint32(buf[76:80], 0) // FWD
	order.PutUint32(buf[80:84], 0) // BWD
	order.PutUint32(buf[84:88], 1) // FREE
	copy(buf[88:96], "LTL-IEEE")

	return buf
}

func TestApiHorizonsRequest(t *testing.T) {
	// This package's TestMain grants NAIFSPK/NAIFLSK download consent once
	// for the whole test binary — remote.Reset() would revoke that for
	// every test that runs afterward (e.g. reader_test.go's TestSPKReader),
	// so restore only the specific override this test makes instead of
	// resetting the whole registry.
	origEndpoint, _ := remote.Lookup(remote.JPLHorizonsSPK)

	t.Cleanup(func() { _ = remote.SetURL(remote.JPLHorizonsSPK, origEndpoint.URL) })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("COMMAND"); got != "'499'" {
			t.Errorf("COMMAND = %q, want '499'", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"Target body name: Mars"}`))
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.JPLHorizonsSPK, srv.URL); err != nil {
		t.Fatal(err)
	}

	start := time.FromJD(2451545.0, time.UTC)
	end := time.FromJD(2451546.0, time.UTC)

	resp, err := apiHorizonsRequest(context.Background(), "499", start, end)
	if err != nil {
		t.Fatalf("apiHorizonsRequest: %v", err)
	}

	if resp.Result != "Target body name: Mars" {
		t.Errorf("Result = %q, want %q", resp.Result, "Target body name: Mars")
	}
}

func TestMapHorizonsStatus(t *testing.T) {
	cases := []struct {
		status int
		want   error
	}{
		{http.StatusBadRequest, ErrHorizonsBadRequest},
		{http.StatusMethodNotAllowed, ErrHorizonsMethodNA},
		{http.StatusInternalServerError, ErrHorizonsServerError},
		{http.StatusServiceUnavailable, ErrHorizonsUnavailable},
	}

	for _, tt := range cases {
		httpErr := &api.HTTPError{StatusCode: tt.status}
		if got := mapHorizonsStatus(httpErr); !errors.Is(got, tt.want) {
			t.Errorf("mapHorizonsStatus(%d) = %v, want %v", tt.status, got, tt.want)
		}
	}

	unexpected := mapHorizonsStatus(&api.HTTPError{StatusCode: http.StatusTeapot})
	if unexpected == nil {
		t.Error("mapHorizonsStatus(teapot) = nil, want ErrHorizonsUnexpected-wrapped error")
	}

	if got := mapHorizonsStatus(remote.ErrOffline); got == nil {
		t.Error("mapHorizonsStatus(non-HTTPError) = nil, want a wrapped error")
	}
}

func TestCacheAPIReusesExistingFile(t *testing.T) {
	bucket := tempBucket(t)

	if err := bucket.WriteAll(context.Background(), "433.bsp", testDAFHeader(), nil); err != nil {
		t.Fatalf("seed kernel object: %v", err)
	}

	start := time.FromJD(2451545.0, time.UTC)
	end := time.FromJD(2451546.0, time.UTC)

	readers, err := CacheAPI(context.Background(), bucket, "", "433", start, end)
	if err != nil {
		t.Fatalf("CacheAPI: %v", err)
	}

	if len(readers) != 1 {
		t.Fatalf("expected 1 reader from the already-cached file, got %d", len(readers))
	}

	if err := readers[0].Close(); err != nil {
		t.Errorf("close: %v", err)
	}
}

func TestCacheAPIGeneratesFromHorizons(t *testing.T) {
	// See TestApiHorizonsRequest: restore only the URL override, never
	// call remote.Reset() (it would revoke TestMain's download consent
	// for the rest of this package's test binary).
	origEndpoint, _ := remote.Lookup(remote.JPLHorizonsSPK)

	t.Cleanup(func() { _ = remote.SetURL(remote.JPLHorizonsSPK, origEndpoint.URL) })

	bucket := tempBucket(t)

	spkB64 := base64.StdEncoding.EncodeToString(testDAFHeader())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"spk_file_id":"generated433","spk":"` + spkB64 + `"}`))
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.JPLHorizonsSPK, srv.URL); err != nil {
		t.Fatal(err)
	}

	remote.EnableDownloads(0, remote.JPLHorizonsSPK)

	start := time.FromJD(2451545.0, time.UTC)
	end := time.FromJD(2451546.0, time.UTC)

	readers, err := CacheAPI(context.Background(), bucket, "", "433", start, end)
	if err != nil {
		t.Fatalf("CacheAPI: %v", err)
	}

	if len(readers) != 1 {
		t.Fatalf("expected 1 generated reader, got %d", len(readers))
	}

	if err := readers[0].Close(); err != nil {
		t.Errorf("close: %v", err)
	}

	if exists, _ := bucket.Exists(context.Background(), "generated433.bsp"); !exists {
		t.Error("expected the generated SPK object to be stored in the bucket")
	}
}

// TestCacheAPIEscalatesToDESForOutOfRangeIDs is a regression test for a
// real bug found via live testing: CacheAPI used to send only a bare
// designation/SPK-ID as COMMAND (e.g. "20000004"), which the real Horizons
// API rejects outright for any ID past the numbered-asteroid record range
// (~895910) — which every real SBDB asteroid SPK-ID (2000000+its number)
// and every comet SPK-ID always is. Horizons documents the fix itself in
// its own error message: wrap it as "DES=<id>;". Confirmed live that this
// silently made every asteroid/comet Stage-2 ephemeris fetch in
// plan.VisibleTonight fail with ErrNoSegment, with no visible error
// (candidateFromTarget treats a construction failure as "skip this
// candidate"), permanently excluding the entire category from every
// result regardless of real brightness.
//
// A real 8-digit SPK-ID is unambiguously out of the numbered-asteroid
// range, so commandCandidates skips the bare attempt entirely and goes
// straight to DES= — see TestCommandCandidates for that decision in
// isolation; this test's mock server would fail loudly (via the "default"
// unmatched-command branch) if CacheAPI ever sent the bare form anyway.
func TestCacheAPIEscalatesToDESForOutOfRangeIDs(t *testing.T) {
	origEndpoint, _ := remote.Lookup(remote.JPLHorizonsSPK)

	t.Cleanup(func() { _ = remote.SetURL(remote.JPLHorizonsSPK, origEndpoint.URL) })

	bucket := tempBucket(t)
	spkB64 := base64.StdEncoding.EncodeToString(testDAFHeader())

	var commands []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cmd := r.URL.Query().Get("COMMAND")
		commands = append(commands, cmd)

		w.Header().Set("Content-Type", "application/json")

		if cmd == "'DES=20000004;'" {
			_, _ = w.Write([]byte(`{"spk_file_id":"generated20000004","spk":"` + spkB64 + `"}`))
			return
		}

		// Any other command (e.g. a bare "20000004" this test doesn't
		// expect CacheAPI to send anymore) gets the real out-of-bounds
		// response shape, so a regression back to trying it would still
		// surface as a request-count mismatch below, not a false pass.
		_, _ = w.Write([]byte(`{"error":"DXREAD: requested IOBJ= 20000004 is out of bounds"}`))
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.JPLHorizonsSPK, srv.URL); err != nil {
		t.Fatal(err)
	}

	remote.EnableDownloads(0, remote.JPLHorizonsSPK)

	start := time.FromJD(2451545.0, time.UTC)
	end := time.FromJD(2451546.0, time.UTC)

	readers, err := CacheAPI(context.Background(), bucket, "", "20000004", start, end)
	if err != nil {
		t.Fatalf("CacheAPI: %v", err)
	}

	defer func() {
		for _, r := range readers {
			_ = r.Close()
		}
	}()

	if len(readers) != 1 {
		t.Fatalf("expected 1 reader from the DES=-escalated request, got %d", len(readers))
	}

	if len(commands) != 1 {
		t.Fatalf("expected exactly 1 request (DES= directly, bare form skipped), got %d: %v", len(commands), commands)
	}

	if commands[0] != "'DES=20000004;'" {
		t.Errorf("COMMAND = %q, want %q", commands[0], "'DES=20000004;'")
	}
}

// TestCacheAPIRetriesWithCAPForComets is a regression test confirming
// CacheAPI's fallback for comets: neither the bare form nor a plain
// "DES=<id>;" match (unlike asteroids) returns a direct SPK for a real
// comet SPK-ID — confirmed live against 1P/Halley (SPK-ID 1000036), which
// returns an epoch-disambiguation table instead. CacheAPI must retry with
// "DES=<id>;CAP" ("closest apparition") before giving up. 1000036 is
// unambiguously out of the numbered-asteroid range, so (like
// TestCacheAPIEscalatesToDESForOutOfRangeIDs) the bare attempt is skipped
// and this starts directly at "DES=<id>;".
func TestCacheAPIRetriesWithCAPForComets(t *testing.T) {
	origEndpoint, _ := remote.Lookup(remote.JPLHorizonsSPK)

	t.Cleanup(func() { _ = remote.SetURL(remote.JPLHorizonsSPK, origEndpoint.URL) })

	bucket := tempBucket(t)
	spkB64 := base64.StdEncoding.EncodeToString(testDAFHeader())

	var commands []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cmd := r.URL.Query().Get("COMMAND")
		commands = append(commands, cmd)

		w.Header().Set("Content-Type", "application/json")

		if strings.HasSuffix(cmd, "CAP'") {
			_, _ = w.Write([]byte(`{"spk_file_id":"generated1000036","spk":"` + spkB64 + `"}`))
			return
		}

		_, _ = w.Write([]byte(`{"result":"Comet AND asteroid index search:\n\n    DES = 1000036;\n\n Matching small-bodies: \n"}`))
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.JPLHorizonsSPK, srv.URL); err != nil {
		t.Fatal(err)
	}

	remote.EnableDownloads(0, remote.JPLHorizonsSPK)

	start := time.FromJD(2451545.0, time.UTC)
	end := time.FromJD(2451546.0, time.UTC)

	readers, err := CacheAPI(context.Background(), bucket, "", "1000036", start, end)
	if err != nil {
		t.Fatalf("CacheAPI: %v", err)
	}

	defer func() {
		for _, r := range readers {
			_ = r.Close()
		}
	}()

	if len(readers) != 1 {
		t.Fatalf("expected 1 reader from the CAP-retried request, got %d", len(readers))
	}

	if len(commands) != 2 {
		t.Fatalf("expected 2 requests (plain DES=, then DES=...;CAP retry — bare form skipped), got %d: %v", len(commands), commands)
	}

	if commands[0] != "'DES=1000036;'" {
		t.Errorf("first COMMAND = %q, want %q", commands[0], "'DES=1000036;'")
	}

	if commands[1] != "'DES=1000036;CAP'" {
		t.Errorf("second COMMAND = %q, want %q", commands[1], "'DES=1000036;CAP'")
	}
}

// TestCommandCandidates locks in the request-skipping decision in
// isolation from CacheAPI's HTTP plumbing.
func TestCommandCandidates(t *testing.T) {
	tests := []struct {
		name   string
		kernel string
		want   []string
	}{
		{"short in-range designation tries bare form first", "433", []string{"433", "DES=433;", "DES=433;CAP"}},
		{"real asteroid SPK-ID skips the doomed bare attempt", "20000004", []string{"DES=20000004;", "DES=20000004;CAP"}},
		{"real comet SPK-ID skips the doomed bare attempt", "1000036", []string{"DES=1000036;", "DES=1000036;CAP"}},
		{"non-numeric designation tries bare form first", "2003 UB313", []string{"2003 UB313", "DES=2003 UB313;", "DES=2003 UB313;CAP"}},
		{"boundary value at the numbered-asteroid ceiling still tries bare", "895910", []string{"895910", "DES=895910;", "DES=895910;CAP"}},
		{"one past the ceiling skips bare", "895911", []string{"DES=895911;", "DES=895911;CAP"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := commandCandidates(tt.kernel)
			if len(got) != len(tt.want) {
				t.Fatalf("commandCandidates(%q) = %v, want %v", tt.kernel, got, tt.want)
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("commandCandidates(%q)[%d] = %q, want %q", tt.kernel, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// tempBucket is a throwaway local bucket standing in for remote's cache.
func tempBucket(t *testing.T) *file.Bucket {
	t.Helper()

	b, err := file.Open(context.Background(), testutil.FileURL(t, t.TempDir()))
	if err != nil {
		t.Fatalf("open bucket: %v", err)
	}

	return b
}
