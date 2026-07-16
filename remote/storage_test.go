package remote

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDataDirOverride(t *testing.T) {
	t.Cleanup(func() {
		SetDataDir("")
		Reset()
	})

	base := t.TempDir()
	SetDataDirPath(base)

	if got := DataDir().LocalPath(); got != base {
		t.Errorf("DataDir = %s, want %s", got, base)
	}

	// Default (unset) resolves under the user cache dir.
	SetDataDir("")

	cacheBase, _ := os.UserCacheDir()
	if want := filepath.Join(cacheBase, "astrogo"); DataDir().LocalPath() != want {
		t.Errorf("default DataDir = %s, want %s", DataDir().LocalPath(), want)
	}
}

func TestCacheDirKindFile(t *testing.T) {
	t.Cleanup(func() {
		SetDataDir("")
		Reset()
	})

	SetDataDirPath(t.TempDir())

	dir, err := CacheDir(NAIFSPK)
	if err != nil {
		t.Fatalf("CacheDir(NAIFSPK): %v", err)
	}

	if !dir.IsDir() {
		t.Errorf("CacheDir should create %s", dir)
	}

	if filepath.Base(dir.LocalPath()) != "jpl" {
		t.Errorf("unexpected cache dir %s, want basename %q", dir, "jpl")
	}
}

func TestCacheDirKindAPIRejected(t *testing.T) {
	t.Cleanup(Reset)

	if _, err := CacheDir(SIMBAD); err == nil {
		t.Error("CacheDir on a KindAPI endpoint should fail")
	}
}

func TestCacheDirUnknownEndpoint(t *testing.T) {
	t.Cleanup(Reset)

	if _, err := CacheDir("no.such.endpoint"); !errors.Is(err, ErrUnknownEndpoint) {
		t.Errorf("expected ErrUnknownEndpoint, got %v", err)
	}
}
