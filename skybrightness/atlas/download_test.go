package atlas

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	gofs "github.com/ungerik/go-fs"

	"github.com/TuSKan/astrogo/remote"
)

// buildSyntheticWorldAtlasZip builds a small in-memory zip archive whose
// single World_Atlas_2015.tif entry is a synthetic GeoTIFF (built with the
// same synthTIFF helper geotiff_test.go's own reader tests use) covering
// EnsureWorldAtlas's central-London validation site — proving
// extract/validate end to end without a multi-gigabyte fixture or any real
// network access.
func buildSyntheticWorldAtlasZip(t *testing.T) []byte {
	t.Helper()

	s := synthTIFF{
		width: 4, height: 4,
		pixels: rampPixels(4, 4, 3), // all-positive mcd/m², no no-data sentinel
		// North-up, top-left at (lon=-10, lat=60), 10° pixels ⇒ covers
		// lon [-10,30], lat [20,60] — includes central London (51.5, -0.13).
		originLon: -10, originLat: 60, pxSize: 10,
	}

	var buf bytes.Buffer

	zw := zip.NewWriter(&buf)

	w, err := zw.Create(worldAtlasZipEntry)
	if err != nil {
		t.Fatalf("zip Create: %v", err)
	}

	if _, err := w.Write(s.build(t)); err != nil {
		t.Fatalf("zip Write: %v", err)
	}

	if err := zw.Close(); err != nil {
		t.Fatalf("zip Close: %v", err)
	}

	return buf.Bytes()
}

// writeFile writes data to dir/name and returns its gofs.File.
func writeFile(t *testing.T, dir, name string, data []byte) gofs.File {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	return gofs.File(path)
}

// TestExtractWorldAtlasTIFF verifies the happy path: the named entry is
// pulled out of the zip and written to destFile, decodable as a GeoTIFF
// afterward.
func TestExtractWorldAtlasTIFF(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	zipFile := writeFile(t, dir, "in.zip", buildSyntheticWorldAtlasZip(t))
	destFile := gofs.File(filepath.Join(dir, "out.tif"))

	if err := extractZIPEntry(zipFile, worldAtlasZipEntry, destFile); err != nil {
		t.Fatalf("extractZIPEntry: %v", err)
	}

	if !destFile.Exists() {
		t.Fatal("extracted file does not exist")
	}

	if err := validateExtractedWorldAtlas(destFile); err != nil {
		t.Errorf("extracted file failed validation: %v", err)
	}
}

// TestExtractWorldAtlasTIFF_MissingEntry verifies a zip lacking the
// expected entry name fails clearly instead of silently extracting nothing.
func TestExtractWorldAtlasTIFF_MissingEntry(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	zw := zip.NewWriter(&buf)

	w, err := zw.Create("some_other_file.tif")
	if err != nil {
		t.Fatalf("zip Create: %v", err)
	}

	if _, err := w.Write([]byte("not the right entry")); err != nil {
		t.Fatalf("zip Write: %v", err)
	}

	if err := zw.Close(); err != nil {
		t.Fatalf("zip Close: %v", err)
	}

	dir := t.TempDir()
	zipFile := writeFile(t, dir, "in.zip", buf.Bytes())
	destFile := gofs.File(filepath.Join(dir, "out.tif"))

	if err := extractZIPEntry(zipFile, worldAtlasZipEntry, destFile); err == nil {
		t.Fatal("expected an error for a zip missing the expected entry, got nil")
	}
}

// TestExtractWorldAtlasTIFF_NotAZip verifies a non-zip file (e.g. a
// truncated or corrupted download) fails clearly.
func TestExtractWorldAtlasTIFF_NotAZip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	zipFile := writeFile(t, dir, "in.zip", []byte("this is definitely not a zip archive"))
	destFile := gofs.File(filepath.Join(dir, "out.tif"))

	if err := extractZIPEntry(zipFile, worldAtlasZipEntry, destFile); err == nil {
		t.Fatal("expected an error for a non-zip file, got nil")
	}
}

// TestValidateExtractedWorldAtlas_Corrupt verifies a garbage (non-GeoTIFF)
// extracted file is rejected via ErrWorldAtlasCorrupt, matching what
// EnsureWorldAtlas checks for before trusting a freshly extracted file.
func TestValidateExtractedWorldAtlas_Corrupt(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	badFile := writeFile(t, dir, "bad.tif", []byte("not a GeoTIFF"))

	err := validateExtractedWorldAtlas(badFile)
	if !errors.Is(err, ErrWorldAtlasCorrupt) {
		t.Fatalf("expected ErrWorldAtlasCorrupt, got %v", err)
	}
}

// worldAtlasTestServer starts an httptest server serving zipBody at
// "/"+worldAtlasZipName, and points remote.WorldAtlas at it — an
// in-process substitute for the real archive host, so this test never
// touches the real network.
func worldAtlasTestServer(t *testing.T, zipBody []byte) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+worldAtlasZipName {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Length", strconv.Itoa(len(zipBody)))
		w.Header().Set("Content-Type", "application/zip")

		if _, err := w.Write(zipBody); err != nil {
			t.Errorf("serve zip: %v", err)
		}
	}))

	t.Cleanup(server.Close)

	if err := remote.SetURL(remote.WorldAtlas, server.URL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}
}

// TestEnsureWorldAtlas_EndToEndOffline exercises the full
// download→extract→validate→cleanup pipeline against an in-process test
// server standing in for the real archive host — proving the wiring works
// without a multi-gigabyte fixture or real network access. It cannot
// exercise EnsureWorldAtlas's "already-extracted, correctly-sized copy ⇒
// skip" fast path, since that check compares against the real archive's
// exact multi-gigabyte worldAtlasExtractedSize, which a synthetic fixture
// this small can never match by construction — that path is covered by
// the network-tagged test instead.
func TestEnsureWorldAtlas_EndToEndOffline(t *testing.T) {
	// Not t.Parallel(): mutates the process-global remote registry/data dir.
	t.Cleanup(remote.Capture().Restore)

	remote.SetDataDir(gofs.File(t.TempDir()))
	worldAtlasTestServer(t, buildSyntheticWorldAtlasZip(t))
	remote.EnableDownloads(remote.WorldAtlas, 0)

	path, err := EnsureWorldAtlas(context.Background())
	if err != nil {
		t.Fatalf("EnsureWorldAtlas: %v", err)
	}

	if !gofs.File(path).Exists() {
		t.Fatalf("EnsureWorldAtlas returned a path that doesn't exist: %s", path)
	}

	// Default WithKeepArchive(false): the downloaded zip must be gone.
	cacheDir, err := remote.CacheDir(remote.WorldAtlas)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	if zipFile := cacheDir.Join(worldAtlasZipName); zipFile.Exists() {
		t.Error("downloaded archive should have been removed after a successful extraction, but it still exists")
	}
}

// TestEnsureWorldAtlas_KeepArchive verifies WithKeepArchive(true) leaves
// the downloaded zip in place.
func TestEnsureWorldAtlas_KeepArchive(t *testing.T) {
	// Not t.Parallel(): mutates the process-global remote registry/data dir.
	t.Cleanup(remote.Capture().Restore)

	remote.SetDataDir(gofs.File(t.TempDir()))
	worldAtlasTestServer(t, buildSyntheticWorldAtlasZip(t))
	remote.EnableDownloads(remote.WorldAtlas, 0)

	if _, err := EnsureWorldAtlas(context.Background(), WithKeepArchive(true)); err != nil {
		t.Fatalf("EnsureWorldAtlas: %v", err)
	}

	cacheDir, err := remote.CacheDir(remote.WorldAtlas)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	if zipFile := cacheDir.Join(worldAtlasZipName); !zipFile.Exists() {
		t.Error("WithKeepArchive(true) should have left the downloaded archive in place")
	}
}

// TestEnsureWorldAtlas_DownloadDenied verifies the consent gate: with no
// remote.EnableDownloads call, EnsureWorldAtlas fails with
// remote.ErrDownloadDenied and never reaches the test server.
func TestEnsureWorldAtlas_DownloadDenied(t *testing.T) {
	// Not t.Parallel(): mutates the process-global remote registry/data dir.
	t.Cleanup(remote.Capture().Restore)

	remote.SetDataDir(gofs.File(t.TempDir()))

	requested := false

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requested = true
	}))
	t.Cleanup(server.Close)

	if err := remote.SetURL(remote.WorldAtlas, server.URL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	_, err := EnsureWorldAtlas(context.Background())
	if !errors.Is(err, remote.ErrDownloadDenied) {
		t.Fatalf("expected remote.ErrDownloadDenied, got %v", err)
	}

	if requested {
		t.Error("test server was contacted despite consent being denied")
	}
}

// TestEnsureWorldAtlas_WithCacheDir verifies WithCacheDir relocates the
// extracted file independently of remote's own cache directory.
func TestEnsureWorldAtlas_WithCacheDir(t *testing.T) {
	// Not t.Parallel(): mutates the process-global remote registry/data dir.
	t.Cleanup(remote.Capture().Restore)

	remote.SetDataDir(gofs.File(t.TempDir()))
	worldAtlasTestServer(t, buildSyntheticWorldAtlasZip(t))
	remote.EnableDownloads(remote.WorldAtlas, 0)

	altDir := t.TempDir()

	path, err := EnsureWorldAtlas(context.Background(), WithCacheDir(altDir))
	if err != nil {
		t.Fatalf("EnsureWorldAtlas: %v", err)
	}

	if filepath.Dir(path) != filepath.Clean(altDir) {
		t.Errorf("extracted file directory = %s, want %s", filepath.Dir(path), altDir)
	}
}
