package spk_test

import (
	"context"
	"encoding/binary"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/jpl/spk"
	"github.com/TuSKan/astrogo/remote"
)

// fakeDAFHeader builds a minimal but structurally valid DAF/SPK file
// record: ND/NI set, FWD/BWD zero (no summary records, so ReadSummaries
// returns an empty list with no error), FREE=1 (so the FREE-derived
// minimum-size check passes trivially), little-endian marker. Exactly
// RecordSize (1024) bytes, matching what NewReader's first ReadAt needs.
func fakeDAFHeader() []byte {
	buf := make([]byte, spk.RecordSize)
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

func TestCacheDownloadDetectsChecksumCorruption(t *testing.T) {
	t.Cleanup(func() {
		remote.SetDataDir("")
		remote.Reset()
	})

	remote.SetDataDirPath(t.TempDir())

	header := fakeDAFHeader()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(header)
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.NAIFSPK, srv.URL); err != nil {
		t.Fatal(err)
	}

	remote.EnableDownloads(remote.NAIFSPK, 0)

	const kernel = "checksum-test.bsp"

	r, err := spk.CacheDownload(context.Background(), kernel)
	if err != nil {
		t.Fatalf("first CacheDownload (bootstraps checksum sidecar): %v", err)
	}

	if err := r.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	cacheDir, err := remote.CacheDir(remote.NAIFSPK)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	kernelFile := cacheDir.Join(kernel)
	sumFile := cacheDir.Join(kernel + ".sha256")

	if !sumFile.Exists() {
		t.Fatal("expected a checksum sidecar to be bootstrapped after the first CacheDownload")
	}

	if err := sumFile.WriteAll([]byte("0000000000000000000000000000000000000000000000000000000000000000")); err != nil {
		t.Fatalf("corrupt sidecar: %v", err)
	}

	_, err = spk.CacheDownload(context.Background(), kernel)
	if !errors.Is(err, spk.ErrCorruptSPK) {
		t.Fatalf("expected ErrCorruptSPK for a checksum mismatch, got %v", err)
	}

	if kernelFile.Exists() {
		t.Error("a checksum-mismatch kernel should have been auto-removed")
	}

	if sumFile.Exists() {
		t.Error("a checksum-mismatch kernel's sidecar should have been auto-removed")
	}
}
