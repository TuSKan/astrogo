package remote_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/remote"
)

// TestNoUndeclaredCleartextEndpoints is the registry guard that keeps a
// cleartext URL from being added without anyone saying why.
//
// # Why every endpoint, not only KindAPI
//
// The obvious reading is that an API request is the sensitive one, because the
// query says what is being observed. But a KindFile fetch over cleartext is at
// least as bad and arguably worse: the bytes coming back are a physical
// constant, a catalogue, or an ephemeris kernel, and anything on the path can
// rewrite them. A tampered luminous-efficiency table does not look like an
// attack, it looks like a slightly different answer. So the rule covers the
// whole registry.
//
// # Why a declared reason rather than a whitelist
//
// A list of exempt IDs rots silently: the entry stays after the upstream gains
// TLS, and nobody re-reads it. A reason recorded on the endpoint has to be
// written by whoever adds the URL, sits next to it, and can be re-checked
// against the service.
func TestNoUndeclaredCleartextEndpoints(t *testing.T) {
	t.Parallel()

	checked := 0

	for _, ep := range remote.Endpoints() {
		id := ep.ID

		u, err := url.Parse(ep.URL)
		if err != nil {
			t.Errorf("%s: URL %q does not parse: %v", id, ep.URL, err)
			continue
		}

		checked++

		if u.Scheme != "http" {
			// https, file, s3 and anything else a blob driver understands are
			// outside this test's remit; only cleartext http is.
			if ep.InsecureReason != "" {
				t.Errorf("%s: scheme is %q but it records an InsecureReason.\n"+
					"  The reason is stale — remove it, or the next reader will "+
					"believe this endpoint is still cleartext.", id, u.Scheme)
			}

			continue
		}

		if strings.TrimSpace(ep.InsecureReason) == "" {
			t.Errorf("%s: %q is cleartext http and records no InsecureReason.\n"+
				"  Switch it to https, or set InsecureReason saying what was "+
				"checked and when. Verify with a real request, not a HEAD: "+
				"SIMBAD and VizieR both answered https identically to http, "+
				"while www.cvrl.org refuses port 443 outright.", id, ep.URL)
		}
	}

	if checked < 20 {
		t.Fatalf("only %d endpoints checked; the registry has shrunk or "+
			"Endpoints() is no longer enumerating it", checked)
	}

	t.Logf("%d endpoints checked", checked)
}

// TestCVRLIsTheOnlyDeclaredCleartextEndpoint pins the current exemption set, so
// adding a second one is a deliberate act that shows up in a diff rather than
// a quiet addition to a growing list.
//
// If an endpoint legitimately needs to join it, extend this test in the same
// change and say why in the commit.
func TestCVRLIsTheOnlyDeclaredCleartextEndpoint(t *testing.T) {
	t.Parallel()

	var exempt []remote.EndpointID

	for _, ep := range remote.Endpoints() {
		if ep.InsecureReason != "" {
			exempt = append(exempt, ep.ID)
		}
	}

	if len(exempt) != 1 || exempt[0] != remote.CVRLLuminosity {
		t.Errorf("cleartext exemptions are %v, want exactly [%v].\n"+
			"  Every addition here ships astrogo's traffic in the clear for "+
			"another service; it needs to be visible in review.",
			exempt, remote.CVRLLuminosity)
	}
}
