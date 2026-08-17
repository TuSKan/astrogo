package remote

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
)

func TestDataDirOverride(t *testing.T) {
	t.Cleanup(func() {
		SetDataDir("")
		Reset()
	})

	want := testutil.FileURL(t, t.TempDir())
	SetDataDir(want)

	if got := DataDirURL(); got != want {
		t.Errorf("DataDirURL = %q, want %q", got, want)
	}

	// Default (unset) resolves to a file:// URL under the user cache dir,
	// built through url.URL so a cache path containing a reserved
	// character still encodes correctly.
	SetDataDir("")

	got := DataDirURL()

	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("default DataDirURL %q does not parse: %v", got, err)
	}

	if u.Scheme != "file" {
		t.Errorf("default DataDirURL scheme = %q, want file", u.Scheme)
	}

	if !strings.HasSuffix(u.Path, "/"+appName) {
		t.Errorf("default DataDirURL path = %q, want it to end in /%s", u.Path, appName)
	}

	if u.Query().Get("create_dir") != "true" {
		t.Errorf("default DataDirURL = %q, want create_dir=true so a first run can create the cache", got)
	}
}

func TestCacheDirKindFile(t *testing.T) {
	t.Cleanup(func() {
		SetDataDir("")
		Reset()
	})

	SetDataDir(testutil.FileURL(t, t.TempDir()))

	bucket, prefix, err := CacheDir(context.Background(), NAIFSPK)
	if err != nil {
		t.Fatalf("CacheDir(NAIFSPK): %v", err)
	}

	if bucket == nil {
		t.Fatal("CacheDir should return a non-nil Bucket")
	}

	if filepath.Base(filepath.Clean(prefix)) != "jpl" {
		t.Errorf("unexpected cache prefix %s, want basename %q", prefix, "jpl")
	}
}

func TestCacheDirKindAPIRejected(t *testing.T) {
	t.Cleanup(Reset)

	if _, _, err := CacheDir(context.Background(), SIMBAD); err == nil {
		t.Error("CacheDir on a KindAPI endpoint should fail")
	}
}

func TestCacheDirUnknownEndpoint(t *testing.T) {
	t.Cleanup(Reset)

	if _, _, err := CacheDir(context.Background(), "no.such.endpoint"); !errors.Is(err, ErrUnknownEndpoint) {
		t.Errorf("expected ErrUnknownEndpoint, got %v", err)
	}
}
