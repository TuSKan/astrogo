package time_test

import (
	"context"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/remote/file"
	"github.com/TuSKan/astrogo/time"

	"github.com/TuSKan/astrogo/internal/testutil"
)

const sampleFinals2000AForGateway = `73 1 2 41684.00 I  0.120733 0.009786  0.136966 0.015902  I 0.8084178 0.0002710  0.0000 0.1916  P    -0.766    0.199    -0.720    0.300   .143000   .137000   .8075000   -18.637    -3.667
73 1 3 41685.00 I  0.118980 0.011039  0.135656 0.013616  I 0.8056163 0.0002710  3.5563 0.1916  P    -0.751    0.199    -0.701    0.300   .141000   .134000   .8044000   -18.636    -3.571  `

// fakeIERSSourceForGateway opens a fresh temp directory as a *file.Bucket,
// points remote.IERSFinals2000A's URL at it, and writes content at
// "finals2000A.all" — the real source object name time/internal/iers's
// fetch.go reads — a local stand-in for an HTTP source now that GetFile
// can't reach an http:// URL at all (no httpblob driver registered yet;
// see remote/file's package doc).
func fakeIERSSourceForGateway(t *testing.T, content string) {
	t.Helper()

	dir := t.TempDir()

	url := testutil.FileURL(t, dir)

	if err := remote.SetURL(remote.IERSFinals2000A, url); err != nil {
		t.Fatal(err)
	}

	bucket, err := file.Open(context.Background(), url)
	if err != nil {
		t.Fatalf("Open fake source: %v", err)
	}

	if err := bucket.WriteAll(context.Background(), "finals2000A.all", []byte(content), nil); err != nil {
		t.Fatalf("seed fake source: %v", err)
	}
}

// TestEOPSourceGateway proves the public gateway actually reaches
// time/internal/iers.EOPSource, covering both the pristine default
// ("zero") and the explicit state RegisterModel sets (see
// RegisterModel's own doc comment on why that distinction matters) --
// ResetEOP is what returns to "zero" afterward, not another
// RegisterModel(ZeroModel{}) call.
func TestEOPSourceGateway(t *testing.T) {
	t.Cleanup(time.ResetEOP)

	if got := time.EOPSource(); got != "zero" {
		t.Errorf(`EOPSource() = %q before any RegisterModel call, want "zero"`, got)
	}

	time.RegisterModel(time.ZeroModel{})

	if got := time.EOPSource(); got != "explicit" {
		t.Errorf(`EOPSource() = %q after RegisterModel, want "explicit"`, got)
	}

	time.ResetEOP()

	if got := time.EOPSource(); got != "zero" {
		t.Errorf(`EOPSource() = %q after ResetEOP, want "zero"`, got)
	}
}

func TestParseFinals2000AGateway(t *testing.T) {
	table, err := time.ParseFinals2000A(strings.NewReader(sampleFinals2000AForGateway))
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
		time.ResetEOP()
		remote.Reset()
	})

	// No source is configured at all (remote.SetURL is never called, so
	// the endpoint keeps its default https:// URL, which has no
	// registered blob driver in this build — see remote/file's package
	// doc) — a non-zero EOP below is therefore proof the pre-seeded
	// cache satisfied the query without ever reaching a fetch attempt.
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	t.Cleanup(func() { remote.SetDataDir("") })

	bucket, prefix, err := remote.CacheDir(context.Background(), remote.IERSFinals2000A)
	if err != nil {
		t.Fatal(err)
	}

	if err := bucket.WriteAll(context.Background(), prefix+"finals2000A.data", []byte(sampleFinals2000AForGateway), nil); err != nil {
		t.Fatal(err)
	}

	tm := time.FromJD(2441684.5, time.UTC) // MJD 41684

	eop := tm.EOP()
	if eop == (time.EOP{}) {
		t.Error("expected non-zero EOP from the pre-seeded cache file")
	}

	lo, hi, ok := time.Coverage()
	if !ok {
		t.Fatal("expected a coverage-reporting model after the lazy load")
	}

	if lo != 41684 || hi != 41685 {
		t.Errorf("Coverage = [%v, %v], want [41684, 41685]", lo, hi)
	}
}

// TestEOPLazyLoadFetchesWithConsent proves the other half of the lazy-load
// contract: with no pre-seeded cache but download consent granted, a bare
// EOP query fetches over the network automatically — no explicit Fetch/
// FetchIfStale call needed.
func TestEOPLazyLoadFetchesWithConsent(t *testing.T) {
	t.Cleanup(func() {
		time.ResetEOP()
		remote.Reset()
		time.SetRetryCooldown(5 * time.Minute)
	})

	// Another test in this binary may have made a recent lazy-load attempt
	// (success or failure); disable the cooldown so this test's own
	// attempt isn't throttled by that unrelated prior attempt.
	time.SetRetryCooldown(0)

	fakeIERSSourceForGateway(t, sampleFinals2000AForGateway)

	remote.EnableDownloads(0, remote.IERSFinals2000A)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	t.Cleanup(func() { remote.SetDataDir("") })

	tm := time.FromJD(2441684.5, time.UTC) // MJD 41684

	eop := tm.EOP()
	if eop == (time.EOP{}) {
		t.Error("expected non-zero EOP after the automatic network fetch")
	}

	if _, _, ok := time.Coverage(); !ok {
		t.Error("expected a coverage-reporting model after the lazy fetch")
	}
}

// TestEOPLazyLoadDegradesToZeroWithoutCacheOrConsent proves the final
// fallback: no pre-seeded cache and no download consent still degrades
// gracefully to a zero EOP, exactly like today, rather than blocking or
// erroring.
func TestEOPLazyLoadDegradesToZeroWithoutCacheOrConsent(t *testing.T) {
	t.Cleanup(func() {
		time.ResetEOP()
		remote.Reset()
	})

	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	t.Cleanup(func() { remote.SetDataDir("") })

	tm := time.FromJD(2441684.5, time.UTC) // MJD 41684

	if eop := tm.EOP(); eop != (time.EOP{}) {
		t.Errorf("expected zero EOP with no cache and no consent, got %+v", eop)
	}
}

func TestSetRetryCooldownGateway(_ *testing.T) {
	// Exercises the gateway wrapper only; time/internal/iers's own tests
	// cover the throttling behavior itself.
	time.SetRetryCooldown(0)
	time.SetRetryCooldown(5 * time.Minute) // restore the default
}
