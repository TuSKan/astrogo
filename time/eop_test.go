package time_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/remote"
	atime "github.com/TuSKan/astrogo/time"
)

const sampleFinals2000AForGateway = `73 1 2 41684.00 I  0.120733 0.009786  0.136966 0.015902  I 0.8084178 0.0002710  0.0000 0.1916  P    -0.766    0.199    -0.720    0.300   .143000   .137000   .8075000   -18.637    -3.667
73 1 3 41685.00 I  0.118980 0.011039  0.135656 0.013616  I 0.8056163 0.0002710  3.5563 0.1916  P    -0.751    0.199    -0.701    0.300   .141000   .134000   .8044000   -18.636    -3.571  `

func TestParseFinals2000AGateway(t *testing.T) {
	table, err := atime.ParseFinals2000A(strings.NewReader(sampleFinals2000AForGateway))
	if err != nil {
		t.Fatalf("ParseFinals2000A: %v", err)
	}

	if _, err := table.EOP(41684); err != nil {
		t.Errorf("expected MJD 41684 to be covered, got: %v", err)
	}
}

// TestEOPLazyLoadFindsPreSeededCacheWithoutConsent proves the core of the
// automatic lazy-load contract: a finals2000A file already sitting at the
// standard cache path (as if hand-copied there for an offline deployment,
// never fetched via remote.GetFile) is found and used by a bare EOP query
// — no remote.EnableDownloads call, no explicit loader call, and (via the
// httptest server below) zero network access.
func TestEOPLazyLoadFindsPreSeededCacheWithoutConsent(t *testing.T) {
	t.Cleanup(func() {
		atime.RegisterModel(atime.ZeroModel{})
		remote.Reset()
	})

	var hits int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(sampleFinals2000AForGateway))
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.IERSFinals2000A, srv.URL); err != nil {
		t.Fatal(err)
	}

	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })

	dir, err := remote.CacheDir(remote.IERSFinals2000A)
	if err != nil {
		t.Fatal(err)
	}

	if err := dir.Join("finals2000A.data").WriteAll([]byte(sampleFinals2000AForGateway)); err != nil {
		t.Fatal(err)
	}

	tm := atime.FromJD(2441684.5, atime.UTC) // MJD 41684

	eop := tm.EOP()
	if eop == (atime.EOP{}) {
		t.Error("expected non-zero EOP from the pre-seeded cache file")
	}

	lo, hi, ok := atime.Coverage()
	if !ok {
		t.Fatal("expected a coverage-reporting model after the lazy load")
	}

	if lo != 41684 || hi != 41685 {
		t.Errorf("Coverage = [%v, %v], want [41684, 41685]", lo, hi)
	}

	if hits != 0 {
		t.Errorf("expected zero network hits when a pre-seeded cache file covers the query, got %d", hits)
	}
}

// TestEOPLazyLoadFetchesWithConsent proves the other half of the lazy-load
// contract: with no pre-seeded cache but download consent granted, a bare
// EOP query fetches over the network automatically — no explicit Fetch/
// FetchIfStale call needed.
func TestEOPLazyLoadFetchesWithConsent(t *testing.T) {
	t.Cleanup(func() {
		atime.RegisterModel(atime.ZeroModel{})
		remote.Reset()
		atime.SetRetryCooldown(5 * atime.Minute)
	})

	// Another test in this binary may have made a recent lazy-load attempt
	// (success or failure); disable the cooldown so this test's own
	// attempt isn't throttled by that unrelated prior attempt.
	atime.SetRetryCooldown(0)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(sampleFinals2000AForGateway))
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.IERSFinals2000A, srv.URL); err != nil {
		t.Fatal(err)
	}

	remote.EnableDownloads(remote.IERSFinals2000A, 0)
	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })

	tm := atime.FromJD(2441684.5, atime.UTC) // MJD 41684

	eop := tm.EOP()
	if eop == (atime.EOP{}) {
		t.Error("expected non-zero EOP after the automatic network fetch")
	}

	if _, _, ok := atime.Coverage(); !ok {
		t.Error("expected a coverage-reporting model after the lazy fetch")
	}
}

// TestEOPLazyLoadDegradesToZeroWithoutCacheOrConsent proves the final
// fallback: no pre-seeded cache and no download consent still degrades
// gracefully to a zero EOP, exactly like today, rather than blocking or
// erroring.
func TestEOPLazyLoadDegradesToZeroWithoutCacheOrConsent(t *testing.T) {
	t.Cleanup(func() {
		atime.RegisterModel(atime.ZeroModel{})
		remote.Reset()
	})

	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })

	tm := atime.FromJD(2441684.5, atime.UTC) // MJD 41684

	if eop := tm.EOP(); eop != (atime.EOP{}) {
		t.Errorf("expected zero EOP with no cache and no consent, got %+v", eop)
	}
}

func TestSetRetryCooldownGateway(_ *testing.T) {
	// Exercises the gateway wrapper only; time/internal/iers's own tests
	// cover the throttling behavior itself.
	atime.SetRetryCooldown(0)
	atime.SetRetryCooldown(5 * atime.Minute) // restore the default
}
