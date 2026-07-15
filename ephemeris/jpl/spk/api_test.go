package spk

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/TuSKan/astrogo/remote"
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
	t.Cleanup(remote.Reset)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("COMMAND"); got != "'499'" {
			t.Errorf("COMMAND = %q, want '499'", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"Target body name: Mars"}`))
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.JPLHorizons, srv.URL); err != nil {
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
		httpErr := &remote.HTTPError{StatusCode: tt.status}
		if got := mapHorizonsStatus(httpErr); !errors.Is(got, tt.want) {
			t.Errorf("mapHorizonsStatus(%d) = %v, want %v", tt.status, got, tt.want)
		}
	}

	unexpected := mapHorizonsStatus(&remote.HTTPError{StatusCode: http.StatusTeapot})
	if unexpected == nil {
		t.Error("mapHorizonsStatus(teapot) = nil, want ErrHorizonsUnexpected-wrapped error")
	}

	if got := mapHorizonsStatus(remote.ErrOffline); got == nil {
		t.Error("mapHorizonsStatus(non-HTTPError) = nil, want a wrapped error")
	}
}

func TestCacheAPIReusesExistingFile(t *testing.T) {
	t.Cleanup(remote.Reset)

	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "433.bsp"), testDAFHeader(), 0o600); err != nil {
		t.Fatalf("seed kernel file: %v", err)
	}

	start := time.FromJD(2451545.0, time.UTC)
	end := time.FromJD(2451546.0, time.UTC)

	readers, err := CacheAPI(context.Background(), "433", start, end, dir)
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
	t.Cleanup(remote.Reset)

	dir := t.TempDir()

	spkB64 := base64.StdEncoding.EncodeToString(testDAFHeader())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"spk_file_id":"generated433","spk":"` + spkB64 + `"}`))
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.JPLHorizons, srv.URL); err != nil {
		t.Fatal(err)
	}

	remote.EnableDownloads(remote.JPLHorizons, 0)

	start := time.FromJD(2451545.0, time.UTC)
	end := time.FromJD(2451546.0, time.UTC)

	readers, err := CacheAPI(context.Background(), "433", start, end, dir)
	if err != nil {
		t.Fatalf("CacheAPI: %v", err)
	}

	if len(readers) != 1 {
		t.Fatalf("expected 1 generated reader, got %d", len(readers))
	}

	if err := readers[0].Close(); err != nil {
		t.Errorf("close: %v", err)
	}

	if !fileExists(filepath.Join(dir, "generated433.bsp")) {
		t.Error("expected the generated SPK file to be saved to disk")
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}
