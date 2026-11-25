package epoch_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/internal/remotetest"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

// finalsRecord is one real finals2000A record. The columns are fixed-width and
// the parser slices by index, so this is copied from the bulletin's own format
// rather than approximated.
const finalsRecord = "73 1 2 41684.00 I  0.120733 0.009786  0.136966 0.015902  " +
	"I 0.8084178 0.0002710  0.0000 0.1916  P    -0.766    0.199    -0.720    0.300"

// bulletin returns n consecutive finals2000A records starting at MJD start,
// each finalsRecord's values under its own date. The parser slices by column,
// so the date and MJD are written at the widths the bulletin uses.
func bulletin(start, n int) string {
	var b strings.Builder

	for i := range n {
		mjd := start + i
		y, m, d := mjdDate(mjd)
		fmt.Fprintf(&b, "%2d%2d%2d %8.2f%s\n", y%100, m, d, float64(mjd), finalsRecord[15:])
	}

	return b.String()
}

// mjdDate is the calendar date of an MJD, for the record's date columns.
func mjdDate(mjd int) (y, m, d int) {
	t := epoch.FromJD(float64(mjd)+2400000.5, epoch.UTC)

	return t.Year(), int(t.Month()), t.Day()
}

// startOver puts the process-wide EOP state and remote's configuration back
// as a test found them, and gives the test an empty cache of its own.
func startOver(t *testing.T) {
	t.Helper()

	t.Cleanup(remote.Capture().Restore)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	epoch.ResetEOP()
	t.Cleanup(epoch.ResetEOP)
}

// serveBulletin points the IERS endpoint at a local directory holding raw as
// finals2000A.all, or nothing if raw is empty.
func serveBulletin(t *testing.T, raw string) {
	t.Helper()

	src := testutil.FileURL(t, t.TempDir())

	if raw != "" {
		fsys, err := remote.OpenFS(t.Context(), src)
		if err != nil {
			t.Fatalf("open the source: %v", err)
		}

		if err := remote.WriteFile(t.Context(), fsys, "finals2000A.all", strings.NewReader(raw)); err != nil {
			t.Fatalf("seed the source: %v", err)
		}
	}

	if err := remote.SetURL(remote.IERSFinals2000A, src); err != nil {
		t.Fatal(err)
	}
}

// cacheBulletin pre-seeds raw where the IERS bulletin is cached.
func cacheBulletin(t *testing.T, raw string) {
	t.Helper()

	fsys, prefix, err := remote.CacheDir(t.Context(), remote.IERSFinals2000A)
	if err != nil {
		t.Fatalf("remote.CacheDir: %v", err)
	}

	if err := remote.WriteFile(t.Context(), fsys, prefix+"finals2000A.data", strings.NewReader(raw)); err != nil {
		t.Fatalf("pre-seed: %v", err)
	}
}

// TestLoadEOPOfflineWithNothingCachedSaysWhy: offline, with the bulletin at
// its own network URL and nothing cached, LoadEOP is an error carrying
// ErrOffline, where the lazy load says nothing and answers zeros (#1187).
func TestLoadEOPOfflineWithNothingCachedSaysWhy(t *testing.T) {
	startOver(t)
	remote.SetOffline(true)

	if err := epoch.LoadEOP(t.Context()); !errors.Is(err, remote.ErrOffline) {
		t.Errorf("LoadEOP offline with nothing cached = %v, want ErrOffline", err)
	}

	if got := epoch.EOPSource(); got != "zero" {
		t.Errorf("EOPSource = %q, want the model left at zero", got)
	}
}

// TestLoadEOPFetchesThenReadsTheCache: LoadEOP takes the network step first
// and the cached bulletin when the network gives none that parses, records
// which in EOPSource, and when neither gives one returns the reason and
// leaves the model as it was.
func TestLoadEOPFetchesThenReadsTheCache(t *testing.T) {
	good := bulletin(61041, 30)

	for _, c := range []struct {
		name           string
		source, cached string
		refuse         bool
		wantSource     string
		wantErr        error
	}{
		{"the network answers", good, "", false, "network", nil},
		{"the download refused, a bulletin cached", good, good, true, "cache", nil},
		{"the network sends junk", "<html>not a bulletin</html>", good, false, "cache", nil},
		{"neither", good, "", true, "zero", remote.ErrDownloadDenied},
	} {
		t.Run(c.name, func(t *testing.T) {
			startOver(t)
			serveBulletin(t, c.source)

			if c.cached != "" {
				cacheBulletin(t, c.cached)
			}

			if c.refuse {
				remotetest.RefuseDownloads(t, remote.IERSFinals2000A)
			}

			err := epoch.LoadEOP(t.Context())

			switch {
			case c.wantErr == nil && err != nil:
				t.Errorf("LoadEOP = %v, want nil", err)
			case c.wantErr != nil && !errors.Is(err, c.wantErr):
				t.Errorf("LoadEOP = %v, want it to carry %v", err, c.wantErr)
			}

			if got := epoch.EOPSource(); got != c.wantSource {
				t.Errorf("EOPSource after LoadEOP = %q, want %q", got, c.wantSource)
			}
		})
	}
}

// TestLoadEOPIsTheModelFromThenOn: the bulletin LoadEOP loads replaces even
// an explicit RegisterModel(ZeroModel{}), answers inside its coverage, and is
// not replaced by the lazy load, which asks the source nothing more for an
// epoch it does not cover.
func TestLoadEOPIsTheModelFromThenOn(t *testing.T) {
	startOver(t)
	epoch.RegisterModel(epoch.ZeroModel{})

	// No cooldown, so the lazy load would ask the source again for an epoch
	// past the bulletin; only the loaded model being the caller's choice
	// stops it. With the default cooldown the throttle LoadEOP leaves behind
	// would stop it as well, and the check below would not tell them apart.
	epoch.SetRetryCooldown(0)
	t.Cleanup(func() { epoch.SetRetryCooldown(5 * time.Minute) })

	raw := bulletin(61041, 30)

	var requests atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)

		_, _ = w.Write([]byte(raw))
	}))
	t.Cleanup(srv.Close)

	if err := remote.SetURL(remote.IERSFinals2000A, srv.URL+"/"); err != nil {
		t.Fatal(err)
	}

	if err := epoch.LoadEOP(t.Context()); err != nil {
		t.Fatalf("LoadEOP: %v", err)
	}

	lo, hi, ok := epoch.Coverage()
	if !ok || lo > 61041 || hi < 61070 {
		t.Fatalf("Coverage after LoadEOP = [%v, %v] %v, want the bulletin's [61041, 61070]", lo, hi, ok)
	}

	if dut1 := epoch.FromJD(61050.5+2400000.5, epoch.UTC).EOP().DUT1; dut1 == 0 {
		t.Errorf("DUT1 inside the bulletin = 0, want the bulletin's")
	}

	asked := requests.Load()

	_ = epoch.FromJD(61500+2400000.5, epoch.UTC).EOP() // past the bulletin

	if more := requests.Load() - asked; more != 0 {
		t.Errorf("an epoch past the loaded bulletin asked the source %d more times, want none", more)
	}

	if got := epoch.EOPSource(); got != "network" {
		t.Errorf("EOPSource = %q, want network", got)
	}
}

// TestLoadEOPHonorsItsContext: the context reaches the download, so a
// canceled load is the cancellation, not a silent fall to zero.
func TestLoadEOPHonorsItsContext(t *testing.T) {
	startOver(t)
	serveBulletin(t, bulletin(61041, 30))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := epoch.LoadEOP(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("LoadEOP with a canceled context = %v, want context.Canceled", err)
	}
}
