package iers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/TuSKan/astrogo/remote"
)

// sampleFinals2000A mimics finals2000A.all format for two consecutive days
// (same fixture shape as reader_test.go's TestParseFinals2000A).
const sampleFinals2000A = `73 1 2 41684.00 I  0.120733 0.009786  0.136966 0.015902  I 0.8084178 0.0002710  0.0000 0.1916  P    -0.766    0.199    -0.720    0.300   .143000   .137000   .8075000   -18.637    -3.667
73 1 3 41685.00 I  0.118980 0.011039  0.135656 0.013616  I 0.8056163 0.0002710  3.5563 0.1916  P    -0.751    0.199    -0.701    0.300   .141000   .134000   .8044000   -18.636    -3.571  `

func TestLoadFile(t *testing.T) {
	t.Cleanup(func() { RegisterModel(ZeroModel{}) })

	path := filepath.Join(t.TempDir(), "finals2000A.all")
	if err := os.WriteFile(path, []byte(sampleFinals2000A), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := LoadFile(path); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	lo, hi, ok := Coverage()
	if !ok {
		t.Fatal("expected a coverage-reporting model after LoadFile")
	}

	if lo != 41684.0 || hi != 41685.0 {
		t.Errorf("Coverage = [%v, %v], want [41684, 41685]", lo, hi)
	}

	if err := LoadFile(filepath.Join(t.TempDir(), "missing.all")); err == nil {
		t.Error("expected an error loading a missing file")
	}
}

func TestLoadFS(t *testing.T) {
	t.Cleanup(func() { RegisterModel(ZeroModel{}) })

	fsys := fstest.MapFS{
		"finals2000A.all": {Data: []byte(sampleFinals2000A)},
	}

	if err := LoadFS(fsys, "finals2000A.all"); err != nil {
		t.Fatalf("LoadFS: %v", err)
	}

	if _, _, ok := Coverage(); !ok {
		t.Error("expected a coverage-reporting model after LoadFS")
	}

	if err := LoadFS(fsys, "does-not-exist.all"); err == nil {
		t.Error("expected an error for a missing FS entry")
	}
}

func TestFetchNow(t *testing.T) {
	t.Cleanup(func() {
		RegisterModel(ZeroModel{})
		remote.Reset()
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(sampleFinals2000A))
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.IERSFinals2000A, srv.URL); err != nil {
		t.Fatal(err)
	}

	// Point the on-disk cache at a scratch dir so this test doesn't read a
	// stale cache file left by another test/run.
	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })

	if err := FetchNow(context.Background()); err != nil {
		t.Fatalf("FetchNow: %v", err)
	}

	if _, _, ok := Coverage(); !ok {
		t.Error("expected a coverage-reporting model after FetchNow")
	}
}

func TestFetchNowHTTPError(t *testing.T) {
	t.Cleanup(func() {
		RegisterModel(ZeroModel{})
		remote.Reset()
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.IERSFinals2000A, srv.URL); err != nil {
		t.Fatal(err)
	}

	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })

	err := FetchNow(context.Background())
	if !errors.Is(err, ErrEOPHTTPStatus) {
		t.Fatalf("expected ErrEOPHTTPStatus, got %v", err)
	}

	if !strings.Contains(err.Error(), strconv.Itoa(http.StatusNotFound)) {
		t.Errorf("expected status code in error, got: %v", err)
	}
}

func TestUseEmbeddedNoDataAvailable(t *testing.T) {
	t.Cleanup(func() { RegisterModel(ZeroModel{}) })

	if len(FinalsData) > 0 {
		t.Skip("this build embeds real finals2000A data (go generate was run) — UseEmbedded's error path isn't reachable here")
	}

	if err := UseEmbedded(); !errors.Is(err, ErrEmbeddedUnavailable) {
		t.Fatalf("expected ErrEmbeddedUnavailable, got %v", err)
	}
}

func TestGetModelLazyLoadDoesNotOverrideExplicitRegistration(t *testing.T) {
	t.Cleanup(func() { RegisterModel(ZeroModel{}) })

	path := filepath.Join(t.TempDir(), "finals2000A.all")
	if err := os.WriteFile(path, []byte(sampleFinals2000A), 0o644); err != nil {
		t.Fatal(err)
	}

	// Explicit registration must win regardless of whether GetModel's lazy
	// embedded-load has already fired in this process.
	if err := LoadFile(path); err != nil {
		t.Fatal(err)
	}

	before, _, _ := Coverage()

	// A further GetModel call must not silently swap the model back to the
	// embedded snapshot.
	_ = GetModel()

	after, _, _ := Coverage()
	if before != after {
		t.Errorf("GetModel call mutated the explicitly-registered model: before=%v after=%v", before, after)
	}
}
