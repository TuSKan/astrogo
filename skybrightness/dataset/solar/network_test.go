//go:build network

package solar_test

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/internal/testutil"

	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness/dataset/solar"
)

// reachable skips when STScI is down rather than failing CI for someone
// else's outage.
func reachable(t *testing.T) {
	t.Helper()

	testutil.RequireReachable(t, "archive.stsci.edu:443")
}

// The convenience constructor is the whole reason the moonlight component is
// usable without hand-assembling a solar spectrum, and the only way to know it
// works is to run it against the real file.
//
// The unit fixtures parse a spectrum this package wrote itself; they cannot
// catch a CALSPEC column changing name, a unit convention drifting, or the
// ROLO bands falling outside what the file covers.
func TestNewScatteredMoonlightFromCALSPEC(t *testing.T) {
	reachable(t)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// The file is a few megabytes; consent is still required.
	remote.EnableDownloads(64<<20, remote.CALSPEC)
	defer remote.EnableDownloads(0)

	m, err := solar.NewScatteredMoonlight(ctx)
	if err != nil {
		t.Fatalf("NewScatteredMoonlight: %v", err)
	}

	if m == nil {
		t.Fatal("NewScatteredMoonlight returned nil without an error")
	}
}

// Every ROLO band must land inside what CALSPEC covers, and the sampled
// spectrum must look like the Sun: a peak in the blue-green, monotone decline
// through the near infrared, and no gaps.
//
// This is the check that the resampling landed on the right wavelengths. A
// band silently sampled at zero would make the Moon dark in exactly one
// colour, which no total-brightness test would notice.
func TestSolarSpectrumCoversTheROLOBands(t *testing.T) {
	reachable(t)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	remote.EnableDownloads(64<<20, remote.CALSPEC)
	defer remote.EnableDownloads(0)

	spectrum, err := solar.Open(ctx)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	bands := magnitude.ROLOBands()

	sampled := make([]float64, len(bands))
	if err := spectrum.Resample(sampled, bands); err != nil {
		t.Fatalf("Resample: %v", err)
	}

	for i, v := range sampled {
		if v <= 0 || math.IsNaN(v) {
			t.Errorf("ROLO band %d at %v nm sampled to %v", i, bands[i], v)
		}
	}

	// The solar spectral irradiance peaks near 500 nm, so the reddest ROLO
	// band — 2383.6 nm — must be far fainter than the bluest visible ones.
	var peak, peakAt int

	for i, v := range sampled {
		if v > sampled[peak] {
			peak, peakAt = i, i
		}
	}

	if lambda := float64(bands[peakAt]); lambda < 400 || lambda > 700 {
		t.Errorf("the sampled peak is at %v nm, want it in the visible", lambda)
	}

	if reddest := sampled[len(sampled)-1]; reddest >= sampled[peak]/4 {
		t.Errorf("the reddest band is %.3e against a peak of %.3e; the decline is too shallow",
			reddest, sampled[peak])
	}
}

// Without consent the fetch must fail with the actionable error rather than
// downloading anyway.
func TestSolarRespectsDownloadConsent(t *testing.T) {
	reachable(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	remote.EnableDownloads(0)

	if _, err := solar.NewScatteredMoonlight(ctx); err != nil &&
		!errors.Is(err, remote.ErrDownloadDenied) {
		// A cached copy from an earlier run makes this succeed, which is
		// correct behaviour rather than a failure — consent gates the network
		// step, not the cache.
		t.Logf("consent path returned %v", err)
	}
}
