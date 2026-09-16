package spk_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io/fs"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/jpl/spk"
	"github.com/TuSKan/astrogo/remote"

	"github.com/TuSKan/astrogo/internal/testutil"
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
	// This package's TestMain grants NAIFSPK/NAIFLSK download consent once
	// for the whole test binary — remote.Reset() would revoke that for
	// every test that runs afterward (e.g. reader_test.go's TestSPKReader),
	// so this captures and restores only NAIFSPK's own state (which also
	// covers the data dir, captured unconditionally by Capture) instead of
	// resetting the whole registry.
	t.Cleanup(remote.Capture(remote.NAIFSPK).Restore)

	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	header := fakeDAFHeader()

	// A local file:// fsys stands in for the source now that GetFile
	// can't reach an http:// URL at all (no httpblob driver registered
	// yet; see remote/file's package doc) — see remote/resume_test.go's
	// fakeSource for the same pattern in package remote itself.
	srcDir := t.TempDir()

	srcURL := testutil.FileURL(t, srcDir)

	srcFS, err := remote.OpenFS(context.Background(), srcURL)
	if err != nil {
		t.Fatalf("Open source: %v", err)
	}

	if err := remote.WriteFile(context.Background(), srcFS, "checksum-test.bsp", bytes.NewReader(header)); err != nil {
		t.Fatalf("seed source: %v", err)
	}

	if err := remote.SetURL(remote.NAIFSPK, srcURL); err != nil {
		t.Fatal(err)
	}

	// Redundant with TestMain's grant, kept for self-containment; restored
	// (not revoked) in cleanup via the Capture above.
	remote.EnableDownloads(0, remote.NAIFSPK)

	const kernel = "checksum-test.bsp"

	r, err := spk.CacheDownload(context.Background(), kernel)
	if err != nil {
		t.Fatalf("first CacheDownload (bootstraps checksum sidecar): %v", err)
	}

	if err := r.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	cacheFS, prefix, err := remote.CacheDir(context.Background(), remote.NAIFSPK)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	kernelKey := prefix + kernel
	sumKey := prefix + kernel + ".sha256"

	// A failed existence check just means "not found" for this assertion.
	if _, err := fs.Stat(cacheFS, sumKey); err != nil {
		t.Fatal("expected a checksum sidecar to be bootstrapped after the first CacheDownload")
	}

	if err := remote.WriteFile(context.Background(), cacheFS, sumKey, bytes.NewReader([]byte("0000000000000000000000000000000000000000000000000000000000000000"))); err != nil {
		t.Fatalf("corrupt sidecar: %v", err)
	}

	_, err = spk.CacheDownload(context.Background(), kernel)
	if !errors.Is(err, spk.ErrCorruptSPK) {
		t.Fatalf("expected ErrCorruptSPK for a checksum mismatch, got %v", err)
	}

	// A failed existence check is not "exists".
	if _, err := fs.Stat(cacheFS, kernelKey); err == nil {
		t.Error("a checksum-mismatch kernel should have been auto-removed")
	}

	if _, err := fs.Stat(cacheFS, sumKey); err == nil {
		t.Error("a checksum-mismatch kernel's sidecar should have been auto-removed")
	}
}
