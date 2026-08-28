package airglow

import (
	"context"

	"github.com/TuSKan/astrogo/remote/file"
)

// CacheLocation exposes where a Spec's skytable is kept, so that a test can
// seed the cache and prove Fetch reads it.
//
// Test-only. The key is a hash of the request rather than of the Spec, which
// is deliberate but does mean nothing outside this package can compute it —
// and a cache nobody can prove is being used is a cache that quietly stops
// being used.
func CacheLocation(ctx context.Context, spec Spec) (*file.Bucket, string, error) {
	req, err := spec.request()
	if err != nil {
		return nil, "", err
	}

	return cacheLocation(ctx, req)
}
