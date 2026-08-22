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

// A KindAPI endpoint has a cache directory like any other.
//
// This test used to assert the opposite, without saying why. What that
// refusal actually did was disable the callers already written against it:
// starlight asks for esa.gaia's directory to checkpoint an hour-long Gaia
// aggregation and resume it across sessions, and both its read and its write
// sit behind an "a cache directory was available" branch that could never be
// taken. The aggregation restarted from nothing every time and nothing
// reported it, because a cache that cannot be reached looks exactly like a
// cold one.
//
// A cache directory is somewhere to put bytes. Whether GetFile can fetch an
// endpoint's content by name from a bucket URL is a separate question, and
// GetFile still answers it with ErrNotFileEndpoint.
func TestCacheDirServesAPIEndpoints(t *testing.T) {
	t.Cleanup(Reset)

	bucket, prefix, err := CacheDir(context.Background(), SIMBAD)
	if err != nil {
		t.Fatalf("CacheDir on a KindAPI endpoint: %v", err)
	}

	if bucket == nil {
		t.Error("CacheDir returned a nil Bucket")
	}

	if prefix == "" || !strings.HasSuffix(prefix, "/") {
		t.Errorf("prefix = %q, want a directory-style prefix", prefix)
	}
}

func TestCacheDirUnknownEndpoint(t *testing.T) {
	t.Cleanup(Reset)

	if _, _, err := CacheDir(context.Background(), "no.such.endpoint"); !errors.Is(err, ErrUnknownEndpoint) {
		t.Errorf("expected ErrUnknownEndpoint, got %v", err)
	}
}

// Every registered endpoint has a cache directory, KindAPI included.
//
// A cache directory is somewhere to put bytes, which is a different question
// from whether GetFile can fetch them by name. CacheDir used to refuse
// KindAPI, and that quietly disabled the callers already written against it:
// starlight asks for esa.gaia's directory to checkpoint an hour-long Gaia
// aggregation and resume it across sessions, and both its read and its write
// sat behind a branch that could never be taken, so the aggregation restarted
// from nothing every time. Nothing reported it, because a cache that cannot be
// reached looks exactly like a cold one.
func TestEveryEndpointHasACacheDirectory(t *testing.T) {
	ctx := context.Background()

	var api, file int

	for id, ep := range defaultEndpoints() {
		bucket, prefix, err := CacheDir(ctx, id)
		if err != nil {
			t.Errorf("CacheDir(%q), kind %v: %v", id, ep.Kind, err)

			continue
		}

		if bucket == nil {
			t.Errorf("CacheDir(%q) returned a nil bucket", id)
		}

		if prefix == "" || !strings.HasSuffix(prefix, "/") {
			t.Errorf("CacheDir(%q) prefix = %q, want a directory-style prefix", id, prefix)
		}

		switch ep.Kind {
		case KindAPI:
			api++
		case KindFile:
			file++
		}
	}

	// Both kinds must actually be represented, or this test passes by
	// covering nothing.
	if api == 0 || file == 0 {
		t.Errorf("the registry offered %d API and %d file endpoints; both kinds must be exercised",
			api, file)
	}
}
