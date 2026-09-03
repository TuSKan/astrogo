package lsk_test

import (
	"context"
	"testing"

	"github.com/TuSKan/astrogo/time"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/ephemeris/jpl/lsk"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

func TestLSKReader(t *testing.T) {
	// Bounded, because remote.NAIFSPK registers a 30-minute DownloadTimeout
	// — right for a caller deliberately fetching a kernel, and longer than
	// this binary's whole budget. Without a cap a stalled fetch hangs until
	// the package times out, which is how a bad minute at NAIF turned main
	// red rather than skipping.
	fetchCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	prov, err := jpl.NewProvider(fetchCtx, core.Planets, "de440s")
	if err != nil {
		testutil.SkipOnUpstreamFailure(t, err)
		t.Fatalf("setup failed: %v", err)
	}

	t.Cleanup(func() {
		err := prov.Close()
		if err != nil {
			t.Errorf("failed to close provider: %v", err)
		}
	})

	ctx := context.Background()

	bucket, prefix, err := remote.CacheDir(ctx, remote.NAIFLSK)
	testutil.AssertNoError(t, err)

	f, err := bucket.NewReader(ctx, prefix+"lsk/naif0012.tls", nil)
	if err != nil {
		t.Fatalf("open cached LSK: %v", err)
	}

	r, err := lsk.NewReader(f)
	testutil.AssertNoError(t, err)

	t.Cleanup(func() {
		err := r.Close()
		if err != nil {
			t.Errorf("failed to close reader: %v", err)
		}
	})

	// Test UTC to TDB conversion
	// Difference between UTC and TDB is roughly 64 seconds + periodic terms at J2000
	j2000 := time.FromJD(2451545.0, time.UTC)
	tdbJD := lsk.UTCToTDB(j2000, r)

	diffSeconds := (tdbJD - j2000.JD()) * 86400.0
	// TDB is ahead of UTC by ~64.184 seconds at J2000
	testutil.AssertNear(t, "TDB-UTC at J2000", diffSeconds, 64.184, 1.0)
}
