package remote

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
)

func TestDefaultEndpointsHaveExplicitTimeouts(t *testing.T) {
	for _, ep := range defaultEndpoints() {
		switch ep.Kind {
		case KindAPI:
			if ep.Timeout == 0 {
				t.Errorf("%s: KindAPI endpoint has no explicit Timeout", ep.ID)
			}
		case KindFile:
			if ep.DownloadTimeout == 0 {
				t.Errorf("%s: KindFile endpoint has no explicit DownloadTimeout", ep.ID)
			}
		}
	}
}

// Endpoint.URL for a KindFile endpoint is a bucket root, and the caller's
// name argument resolves within it. A URL naming one exact object cannot
// resolve a name at all, and does so silently — this asserts the
// convention every KindFile entry must follow.
func TestKindFileEndpointsAreDirectoryPrefixes(t *testing.T) {
	for _, ep := range defaultEndpoints() {
		if ep.Kind != KindFile {
			continue
		}

		u, err := url.Parse(ep.URL)
		if err != nil {
			t.Errorf("%s: URL %q does not parse: %v", ep.ID, ep.URL, err)

			continue
		}

		// A bucket-scheme URL addresses the bucket itself and has no path
		// to end in a slash; only a path-bearing source needs the check.
		if u.Path != "" && !strings.HasSuffix(u.Path, "/") {
			t.Errorf("%s: URL %q is not a directory-style prefix", ep.ID, ep.URL)
		}
	}
}

// The Copernicus endpoint reaches a non-AWS S3 service, and everything the
// driver needs to do that lives in the URL rather than in Go. If these
// params are lost or mistyped the failure is a confusing AWS-shaped error
// at first read, so they are asserted here.
func TestCopernicusURLCarriesS3ConnectionParams(t *testing.T) {
	u, err := url.Parse(defaultEndpoints()[CopernicusEODATA].URL)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if u.Scheme != "s3" || u.Host != "eodata" {
		t.Errorf("bucket = %s://%s, want s3://eodata", u.Scheme, u.Host)
	}

	want := map[string]string{
		"endpoint":           "https://eodata.dataspace.copernicus.eu",
		"hostname_immutable": "true",
		"region":             "default",
		"use_path_style":     "true",
	}

	for k, v := range want {
		if got := u.Query().Get(k); got != v {
			t.Errorf("query %q = %q, want %q", k, got, v)
		}
	}
}

func TestDefaultEndpointsCacheDirsMatchOnDiskLayout(t *testing.T) {
	t.Cleanup(func() {
		SetDataDir("")
		Reset()
	})

	SetDataDir(testutil.FileURL(t, t.TempDir()))

	want := map[EndpointID]string{
		IERSFinals2000A: "iers/",
		NAIFSPK:         "jpl/",
		NAIFLSK:         "jpl/",
		OpenNGC:         "openngc/",
	}

	for id, prefix := range want {
		_, got, err := CacheDir(context.Background(), id)
		if err != nil {
			t.Fatalf("CacheDir(%s): %v", id, err)
		}

		if got != prefix {
			t.Errorf("CacheDir(%s) prefix = %q, want %q", id, got, prefix)
		}
	}
}
