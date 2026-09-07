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

// TestNoAPIEndpointRequiresACredential is what internal/testutil's
// SkipOnUpstreamFailure treats a 403 as the upstream's problem on the strength
// of.
//
// # The argument it holds up
//
// A 403 says "I know who you are and I decline". To a caller that sent no
// credential there is nothing about the request to correct, so it is the
// service's policy rather than astrogo's defect — CelesTrak answers a burst
// that way and serves the same query normally a minute later (#206). A 401 is
// the opposite and stays a failure: it says the service wants authentication,
// which for an endpoint astrogo believes is public means the endpoint moved or
// grew a requirement.
//
// That reasoning is only sound while astrogo never sends a credential the
// service demands. TokenEnv's own contract says so — "a token is an
// optimisation, never a requirement: every endpoint that declares one must
// still work without it" — and this is where the contract stops being prose.
//
// # Why the list rather than a flag
//
// An inventory, not an exclusion list: adding an entry means writing down why
// the endpoint needs a token and confirming it still answers without one. A
// boolean would be set without either.
//
// The KindAPI restriction is the other half. Credentials also reach
// CopernicusEODATA, through the AWS SDK's default chain, and a 403 from there
// is a real configuration error a developer needs to see. It never reaches the
// classifier: SkipOnUpstreamFailure matches errors carrying an HTTPStatus, and
// only remote/api's HTTPError has one — a blob-backed endpoint's failures come
// from gocloud and the AWS SDK instead. So the invariant is specifically that
// no *KindAPI* endpoint requires a credential.
func TestNoAPIEndpointRequiresACredential(t *testing.T) {
	// Every KindAPI endpoint that declares a token, and why it is optional
	// there. Gaia@AIP serves the same DR3 tables unauthenticated; the token
	// only raises the row limit and the queue priority.
	optional := map[EndpointID]string{
		GaiaAIP:      "Gaia@AIP serves the same DR3 tables anonymously; a token raises row limits and queue priority",
		GaiaAIPAsync: "the same mirror's async job endpoint, and the same token, with the same anonymous fallback",
	}

	for _, ep := range Endpoints() {
		if ep.TokenEnv == "" {
			continue
		}

		if ep.Kind != KindAPI {
			continue
		}

		if _, ok := optional[ep.ID]; !ok {
			t.Errorf("endpoint %s declares TokenEnv %q and is not in this test's inventory.\n"+
				"  If the token is optional, add it with the reason. If the service now *requires*\n"+
				"  one, internal/testutil.SkipOnUpstreamFailure must stop treating 403 as the\n"+
				"  upstream's problem — it would hide a real authorization failure.",
				ep.ID, ep.TokenEnv)
		}
	}

	// The inventory must not outlive its entries either, or it stops being a
	// record of anything.
	for id := range optional {
		ep, ok := Lookup(id)
		if !ok {
			t.Errorf("inventory names %s, which is not a registered endpoint", id)
			continue
		}

		if ep.TokenEnv == "" {
			t.Errorf("inventory names %s as token-carrying, but it declares no TokenEnv", id)
		}
	}
}
