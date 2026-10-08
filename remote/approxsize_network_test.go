//go:build network

package remote_test

import (
	"context"
	"io/fs"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

// TestListedFilesFitTheirApproxSize holds every downloadable endpoint that
// lists a fixed file manifest to a size budget that covers each file it lists.
//
// Consent is checked against an endpoint's ApproxSize before a request and
// against the size the source reports after it, and the documented way to
// grant it is EnableDownloads(ApproxSize, ...). So a file even a few bytes
// larger than its endpoint's ApproxSize is refused to every caller who follows
// that recipe. Two were (#659): each SFD hemisphere is 67,115,520 bytes against
// a budget of 64 MiB, its FITS header over, and OpenNGC's NGC.csv is 3,876,288
// bytes against 2,000,000.
func TestListedFilesFitTheirApproxSize(t *testing.T) {
	for _, ep := range remote.Endpoints() {
		if !ep.Enabled || !ep.Downloadable || len(ep.Files) == 0 || ep.ApproxSize <= 0 {
			continue
		}

		t.Run(string(ep.ID), func(t *testing.T) {
			u, err := url.Parse(ep.URL)
			if err != nil {
				t.Fatalf("parse %s: %v", ep.URL, err)
			}

			port := u.Port()
			if port == "" {
				port = map[string]string{"http": "80", "https": "443"}[u.Scheme]
			}

			if host := net.JoinHostPort(u.Hostname(), port); !testutil.Reachable(host) {
				t.Skipf("%s is unreachable", host)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			fsys, err := remote.OpenFS(ctx, ep.URL)
			if err != nil {
				t.Fatalf("OpenFS(%s): %v", ep.URL, err)
			}

			for _, name := range ep.Files {
				info, err := fs.Stat(fsys, name)
				if err != nil {
					testutil.SkipOnUpstreamFailure(t, err)
					t.Fatalf("Stat(%s): %v", name, err)
				}

				if info.Size() > ep.ApproxSize {
					t.Errorf("%s is %d bytes and %s's ApproxSize is %d: a grant of ApproxSize refuses it",
						name, info.Size(), ep.ID, ep.ApproxSize)
				}
			}
		})
	}
}
