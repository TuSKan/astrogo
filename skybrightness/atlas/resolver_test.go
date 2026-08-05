package atlas_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gofs "github.com/ungerik/go-fs"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/skybrightness/atlas"
	"github.com/TuSKan/astrogo/skybrightness/lpmap"
)

// lpmapTestServer starts an in-process fake lightpollutionmap.info
// QueryRaster server returning artMcdM2 (World Atlas mcd/m² units) for
// every query, and points remote.LightPollution at it — the same pattern
// skybrightness/lpmap's own test suite uses. Restored via t.Cleanup by the
// caller (remote.Capture(...).Restore).
func lpmapTestServer(t *testing.T, artMcdM2 float64) *lpmap.Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := fmt.Fprintf(w, "-46.6333,-23.5505,%v", artMcdM2); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	t.Cleanup(server.Close)

	if err := remote.SetURL(remote.LightPollution, server.URL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	return lpmap.New(lpmap.WithAPIKey("test-key"), lpmap.WithHTTPClient(server.Client()))
}

// testSite is the geodetic position every Resolver test samples — São
// Paulo. Note coord.NewGeodetic's argument order is (lon, lat, height),
// east-positive.
func testSite(t *testing.T) *coord.Geodetic {
	t.Helper()

	loc, err := coord.NewGeodetic(angle.Deg(-46.6333), angle.Deg(-23.5505), 0)
	if err != nil {
		t.Fatalf("build test location: %v", err)
	}

	return loc
}

// denyAllDownloads points remote.WorldAtlas/remote.VIIRSAnnual at a fresh
// temp cache dir with no consent granted, so LayerAuto's always-attempted
// WorldAtlas/VIIRS legs fail fast (remote.ErrDownloadDenied) without any
// network access — the deterministic "nothing configured to download"
// baseline every LayerAuto test in this file builds on.
func denyAllDownloads(t *testing.T) {
	t.Helper()
	t.Cleanup(remote.Capture(remote.WorldAtlas, remote.VIIRSAnnual).Restore)
	remote.SetDataDir(gofs.File(t.TempDir()))
}

// TestResolver_NoLayerConfigured verifies LayerAuto with nothing but the
// always-attempted download legs (both denied, no network) returns
// ErrNoTierAvailable with both attempts recorded.
func TestResolver_NoLayerConfigured(t *testing.T) {
	denyAllDownloads(t)

	r := atlas.NewResolver()
	defer func() { _ = r.Close() }()

	result, err := r.Floor(context.Background(), testSite(t))
	if !errors.Is(err, atlas.ErrNoTierAvailable) {
		t.Fatalf("expected ErrNoTierAvailable, got %v", err)
	}

	if len(result.Attempts) != 2 {
		t.Errorf("Attempts = %+v, want 2 (VIIRS denied, WorldAtlas denied)", result.Attempts)
	}

	if !errors.Is(err, remote.ErrDownloadDenied) {
		t.Errorf("expected remote.ErrDownloadDenied reachable via errors.Is, got %v", err)
	}
}

// TestResolver_ExplicitLayerScalar verifies WithLayer(LayerScalar)
// resolves directly to the configured fixed brightness, with no other
// layer attempted.
func TestResolver_ExplicitLayerScalar(t *testing.T) {
	t.Parallel()

	const sqm = skybrightness.SurfaceBrightnessV(19.5)

	r := atlas.NewResolver(atlas.WithLayer(atlas.LayerScalar), atlas.WithScalarSQM(sqm))
	defer func() { _ = r.Close() }()

	result, err := r.Floor(context.Background(), testSite(t))
	if err != nil {
		t.Fatalf("Floor: %v", err)
	}

	if result.Layer != atlas.LayerScalar {
		t.Errorf("Layer = %v, want LayerScalar", result.Layer)
	}

	if result.SQM != sqm {
		t.Errorf("SQM = %v, want %v", result.SQM, sqm)
	}

	if len(result.Attempts) != 1 || result.Attempts[0].Layer != atlas.LayerScalar || result.Attempts[0].Err != nil {
		t.Errorf("Attempts = %+v, want exactly one successful LayerScalar attempt", result.Attempts)
	}
}

// TestResolver_ExplicitLayerScalarNotConfigured verifies choosing
// LayerScalar without WithScalarSQM fails clearly instead of silently
// falling through to anything else.
func TestResolver_ExplicitLayerScalarNotConfigured(t *testing.T) {
	t.Parallel()

	r := atlas.NewResolver(atlas.WithLayer(atlas.LayerScalar))
	defer func() { _ = r.Close() }()

	_, err := r.Floor(context.Background(), testSite(t))
	if !errors.Is(err, atlas.ErrScalarNotConfigured) {
		t.Fatalf("expected ErrScalarNotConfigured, got %v", err)
	}
}

// TestResolver_ExplicitLayerBortle verifies WithLayer(LayerBortle)
// resolves to skybrightness.FloorFromBortle's value for the configured
// class.
func TestResolver_ExplicitLayerBortle(t *testing.T) {
	t.Parallel()

	r := atlas.NewResolver(atlas.WithLayer(atlas.LayerBortle), atlas.WithBortleClass(4))
	defer func() { _ = r.Close() }()

	result, err := r.Floor(context.Background(), testSite(t))
	if err != nil {
		t.Fatalf("Floor: %v", err)
	}

	if result.Layer != atlas.LayerBortle {
		t.Errorf("Layer = %v, want LayerBortle", result.Layer)
	}

	want, _ := skybrightness.FloorFromBortle(4)
	wantNL, _ := want.Radiance(coord.NewAltAz(angle.Deg(90), angle.Zero()), nil)

	if want := wantNL.SurfaceBrightnessV(); result.SQM != want {
		t.Errorf("SQM = %v, want %v", result.SQM, want)
	}
}

// TestResolver_ExplicitLayerBortleInvalidClass verifies an out-of-range
// Bortle class surfaces skybrightness.ErrBortleClass directly — with a
// single explicit Layer there is nothing to fall through to.
func TestResolver_ExplicitLayerBortleInvalidClass(t *testing.T) {
	t.Parallel()

	r := atlas.NewResolver(atlas.WithLayer(atlas.LayerBortle), atlas.WithBortleClass(99))
	defer func() { _ = r.Close() }()

	_, err := r.Floor(context.Background(), testSite(t))
	if !errors.Is(err, skybrightness.ErrBortleClass) {
		t.Fatalf("expected skybrightness.ErrBortleClass, got %v", err)
	}
}

// TestResolver_ExplicitLayerLightPollutionMapSucceeds verifies
// WithLayer(LayerLightPollutionMap) resolves via a live-shaped (httptest)
// query, and that its own artificial-only Floor is returned untouched.
func TestResolver_ExplicitLayerLightPollutionMapSucceeds(t *testing.T) {
	// Not t.Parallel(): mutates the process-global remote registry.
	t.Cleanup(remote.Capture(remote.LightPollution).Restore)

	const artMcdM2 = 6.64

	client := lpmapTestServer(t, artMcdM2)

	r := atlas.NewResolver(atlas.WithLayer(atlas.LayerLightPollutionMap), atlas.WithLightPollutionMap(client))
	defer func() { _ = r.Close() }()

	result, err := r.Floor(context.Background(), testSite(t))
	if err != nil {
		t.Fatalf("Floor: %v", err)
	}

	if result.Layer != atlas.LayerLightPollutionMap {
		t.Errorf("Layer = %v, want LayerLightPollutionMap", result.Layer)
	}

	want := skybrightness.SurfaceBrightnessFromMcdM2(artMcdM2)
	if diff := float64(result.SQM) - float64(want); diff > 1e-9 || diff < -1e-9 {
		t.Errorf("SQM = %v, want %v", result.SQM, want)
	}
}

// TestResolver_ExplicitLayerLightPollutionMapNotConfigured verifies
// choosing LayerLightPollutionMap without WithLightPollutionMap fails
// clearly.
func TestResolver_ExplicitLayerLightPollutionMapNotConfigured(t *testing.T) {
	t.Parallel()

	r := atlas.NewResolver(atlas.WithLayer(atlas.LayerLightPollutionMap))
	defer func() { _ = r.Close() }()

	_, err := r.Floor(context.Background(), testSite(t))
	if !errors.Is(err, atlas.ErrLightPollutionMapNotConfigured) {
		t.Fatalf("expected ErrLightPollutionMapNotConfigured, got %v", err)
	}
}

// TestResolver_ExplicitLayerWorldAtlasDeniedNoFallback verifies
// WithLayer(LayerWorldAtlas) with no download consent fails with
// remote.ErrDownloadDenied directly — a single explicit Layer never
// falls through to VIIRS/LPMap/Bortle/Scalar even if they'd be
// available under LayerAuto.
func TestResolver_ExplicitLayerWorldAtlasDeniedNoFallback(t *testing.T) {
	denyAllDownloads(t)

	r := atlas.NewResolver(
		atlas.WithLayer(atlas.LayerWorldAtlas),
		atlas.WithBortleClass(4), // configured but must be ignored: not LayerAuto
	)
	defer func() { _ = r.Close() }()

	_, err := r.Floor(context.Background(), testSite(t))
	if !errors.Is(err, remote.ErrDownloadDenied) {
		t.Fatalf("expected remote.ErrDownloadDenied, got %v", err)
	}
}

// TestResolver_ExplicitLayerVIIRSDeniedDefaultYear verifies
// WithLayer(LayerVIIRS) with no download consent and no WithVIIRSYear
// still fails cleanly with remote.ErrDownloadDenied (not
// ErrVIIRSYearOutOfRange) — proving the default year
// (atlas.LatestVIIRSYear) is valid on its own.
func TestResolver_ExplicitLayerVIIRSDeniedDefaultYear(t *testing.T) {
	denyAllDownloads(t)

	r := atlas.NewResolver(atlas.WithLayer(atlas.LayerVIIRS))
	defer func() { _ = r.Close() }()

	_, err := r.Floor(context.Background(), testSite(t))
	if !errors.Is(err, remote.ErrDownloadDenied) {
		t.Fatalf("expected remote.ErrDownloadDenied, got %v", err)
	}
}

// TestResolver_AutoFallsThroughToLightPollutionMap verifies LayerAuto,
// with download consent denied but a light-pollution-map client
// configured, tries VIIRS then WorldAtlas (both denied) then succeeds at
// LayerLightPollutionMap — the exact scenario from the user's original
// examples/18_sky_brightness report ("Light-pollution floor unavailable
// ... using natural sky only"), now resolved instead of dead-ending.
//
// The VIIRS-before-WorldAtlas ordering is pinned deliberately: autoOrder
// is freshness-first, and silently flipping it would change which source
// every default caller gets.
func TestResolver_AutoFallsThroughToLightPollutionMap(t *testing.T) {
	denyAllDownloads(t)
	t.Cleanup(remote.Capture(remote.LightPollution).Restore)

	const artMcdM2 = 6.64

	client := lpmapTestServer(t, artMcdM2)

	r := atlas.NewResolver(atlas.WithLightPollutionMap(client))
	defer func() { _ = r.Close() }()

	result, err := r.Floor(context.Background(), testSite(t))
	if err != nil {
		t.Fatalf("Floor: %v", err)
	}

	if result.Layer != atlas.LayerLightPollutionMap {
		t.Errorf("Layer = %v, want LayerLightPollutionMap", result.Layer)
	}

	if len(result.Attempts) != 3 {
		t.Fatalf("Attempts = %+v, want 3 (VIIRS denied, WorldAtlas denied, LightPollutionMap succeeded)", result.Attempts)
	}

	if result.Attempts[0].Layer != atlas.LayerVIIRS || !errors.Is(result.Attempts[0].Err, remote.ErrDownloadDenied) {
		t.Errorf("Attempts[0] = %+v, want a denied LayerVIIRS attempt", result.Attempts[0])
	}

	if result.Attempts[1].Layer != atlas.LayerWorldAtlas || !errors.Is(result.Attempts[1].Err, remote.ErrDownloadDenied) {
		t.Errorf("Attempts[1] = %+v, want a denied LayerWorldAtlas attempt", result.Attempts[1])
	}

	if result.Attempts[2].Layer != atlas.LayerLightPollutionMap || result.Attempts[2].Err != nil {
		t.Errorf("Attempts[2] = %+v, want a successful LayerLightPollutionMap attempt", result.Attempts[2])
	}
}

// TestResolver_AutoFallsThroughToScalar verifies LayerAuto with only
// WithScalarSQM configured falls all the way through VIIRS/WorldAtlas
// (both denied) to the fixed scalar fallback.
func TestResolver_AutoFallsThroughToScalar(t *testing.T) {
	denyAllDownloads(t)

	const scalarSQM = skybrightness.SurfaceBrightnessV(20.0)

	r := atlas.NewResolver(atlas.WithScalarSQM(scalarSQM))
	defer func() { _ = r.Close() }()

	result, err := r.Floor(context.Background(), testSite(t))
	if err != nil {
		t.Fatalf("Floor: %v", err)
	}

	if result.Layer != atlas.LayerScalar {
		t.Errorf("Layer = %v, want LayerScalar", result.Layer)
	}

	if result.SQM != scalarSQM {
		t.Errorf("SQM = %v, want %v", result.SQM, scalarSQM)
	}

	if len(result.Attempts) != 3 {
		t.Errorf("Attempts = %+v, want 3 (VIIRS denied, WorldAtlas denied, Scalar succeeded)", result.Attempts)
	}
}

// TestResolver_AutoSkipsUnconfiguredLightPollutionMapAndBortle verifies
// LayerAuto never attempts (and fails) LayerLightPollutionMap/LayerBortle
// when their own With* option was never given — only genuinely-attempted
// legs appear in Attempts.
func TestResolver_AutoSkipsUnconfiguredLightPollutionMapAndBortle(t *testing.T) {
	denyAllDownloads(t)

	const scalarSQM = skybrightness.SurfaceBrightnessV(21.0)

	r := atlas.NewResolver(atlas.WithScalarSQM(scalarSQM))
	defer func() { _ = r.Close() }()

	result, err := r.Floor(context.Background(), testSite(t))
	if err != nil {
		t.Fatalf("Floor: %v", err)
	}

	for _, a := range result.Attempts {
		if a.Layer == atlas.LayerLightPollutionMap || a.Layer == atlas.LayerBortle {
			t.Errorf("unconfigured layer %v should have been skipped, not attempted: %+v", a.Layer, a)
		}
	}
}

// TestResolver_JoinedErrorsResolveEveryLayer verifies that when every
// attempted layer fails, the returned error wraps ErrNoTierAvailable AND
// every individual layer's own error is still reachable via errors.Is —
// so a caller can distinguish "denied download consent" from "invalid
// Bortle class" from a generic aggregate failure.
func TestResolver_JoinedErrorsResolveEveryLayer(t *testing.T) {
	denyAllDownloads(t)

	r := atlas.NewResolver(atlas.WithBortleClass(99)) // invalid class
	defer func() { _ = r.Close() }()

	_, err := r.Floor(context.Background(), testSite(t))

	if !errors.Is(err, atlas.ErrNoTierAvailable) {
		t.Errorf("expected errors.Is(err, ErrNoTierAvailable), got %v", err)
	}

	if !errors.Is(err, remote.ErrDownloadDenied) {
		t.Errorf("expected WorldAtlas/VIIRS's own error (remote.ErrDownloadDenied) to survive the join: %v", err)
	}

	if !errors.Is(err, skybrightness.ErrBortleClass) {
		t.Errorf("expected the Bortle layer's own error (skybrightness.ErrBortleClass) to survive the join: %v", err)
	}
}

// TestResolver_AtlasFileMissingFallsThrough verifies a configured but
// unreachable/invalid WithAtlasFile is recorded as a failed
// LayerWorldAtlas attempt, not a hard error — the ladder proceeds to the
// next layer. This exercises WithAtlasFile without needing a real,
// decodable GeoTIFF fixture (this package's own internal tests already
// cover successful decoding).
func TestResolver_AtlasFileMissingFallsThrough(t *testing.T) {
	denyAllDownloads(t)

	const scalarSQM = skybrightness.SurfaceBrightnessV(21.5)

	r := atlas.NewResolver(
		atlas.WithAtlasFile("/nonexistent/path/to/atlas.tif"),
		atlas.WithScalarSQM(scalarSQM),
	)
	defer func() { _ = r.Close() }()

	result, err := r.Floor(context.Background(), testSite(t))
	if err != nil {
		t.Fatalf("Floor: %v", err)
	}

	if result.Layer != atlas.LayerScalar {
		t.Errorf("Layer = %v, want LayerScalar", result.Layer)
	}

	// WorldAtlas is the SECOND rung under the freshness-first autoOrder
	// (VIIRS leads); index 1, not 0.
	if len(result.Attempts) != 3 || result.Attempts[1].Layer != atlas.LayerWorldAtlas || result.Attempts[1].Err == nil {
		t.Errorf("Attempts = %+v, want a failed LayerWorldAtlas attempt second", result.Attempts)
	}

	// A second Floor call must reuse the cached (failed) open attempt
	// rather than re-touch the filesystem — same failure, same message.
	result2, err2 := r.Floor(context.Background(), testSite(t))
	if err2 != nil {
		t.Fatalf("second Floor call: %v", err2)
	}

	if result2.Attempts[1].Err.Error() != result.Attempts[1].Err.Error() {
		t.Errorf("second call's cached WorldAtlas error = %q, want the same as the first call's %q",
			result2.Attempts[1].Err, result.Attempts[1].Err)
	}
}

// TestResolver_CloseIsIdempotentAndSafeWithoutUse verifies Close never
// panics, whether or not any resource-holding layer was ever configured
// or used, and tolerates being called more than once.
func TestResolver_CloseIsIdempotentAndSafeWithoutUse(t *testing.T) {
	t.Parallel()

	r := atlas.NewResolver(atlas.WithLayer(atlas.LayerScalar), atlas.WithScalarSQM(20.0))

	if err := r.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}

	if err := r.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// TestFloorAt_OneShot verifies the package-level one-shot resolves the
// same value as a hand-built Resolver, and — the point of the helper —
// leaves nothing for the caller to close.
func TestFloorAt_OneShot(t *testing.T) {
	t.Parallel()

	const sqm = skybrightness.SurfaceBrightnessV(19.5)

	result, err := atlas.FloorAt(context.Background(), testSite(t),
		atlas.WithLayer(atlas.LayerScalar), atlas.WithScalarSQM(sqm))
	if err != nil {
		t.Fatalf("FloorAt: %v", err)
	}

	if result.Layer != atlas.LayerScalar {
		t.Errorf("Layer = %v, want LayerScalar", result.Layer)
	}

	if result.SQM != sqm {
		t.Errorf("SQM = %v, want %v", result.SQM, sqm)
	}

	// The returned Floor must stay usable after FloorAt closed its
	// internal Resolver — every layer yields a scalar Floor holding no
	// file reference.
	nl, err := result.Floor.Radiance(coord.NewAltAz(angle.Deg(90), angle.Zero()), nil)
	if err != nil {
		t.Fatalf("sample returned Floor after FloorAt returned: %v", err)
	}

	if got := nl.SurfaceBrightnessV(); got != sqm {
		t.Errorf("resampled Floor = %v, want %v", got, sqm)
	}
}

// TestFloor_NilLocation verifies both entry points reject a nil location
// with ErrNilLocation rather than dereferencing it.
func TestFloor_NilLocation(t *testing.T) {
	t.Parallel()

	r := atlas.NewResolver(atlas.WithLayer(atlas.LayerScalar), atlas.WithScalarSQM(20.0))
	defer func() { _ = r.Close() }()

	if _, err := r.Floor(context.Background(), nil); !errors.Is(err, atlas.ErrNilLocation) {
		t.Errorf("Resolver.Floor(nil): got %v, want ErrNilLocation", err)
	}

	_, err := atlas.FloorAt(context.Background(), nil, atlas.WithLayer(atlas.LayerScalar), atlas.WithScalarSQM(20.0))
	if !errors.Is(err, atlas.ErrNilLocation) {
		t.Errorf("FloorAt(nil): got %v, want ErrNilLocation", err)
	}
}

// viirsYearServer stands in for remote.VIIRSAnnual, answering HEAD only
// for the years listed in have. It records every year probed so a test can
// assert the walk stopped at the first gap instead of scanning blindly.
func viirsYearServer(t *testing.T, have map[int]bool) *[]string {
	t.Helper()

	var probed []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		probed = append(probed, name)

		for year, ok := range have {
			if ok && name == fmt.Sprintf("viirs_%d_raw.zip", year) {
				w.Header().Set("Content-Length", "1024")
				w.WriteHeader(http.StatusOK)

				return
			}
		}

		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	if err := remote.SetURL(remote.VIIRSAnnual, srv.URL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	return &probed
}

// TestNewestVIIRSYear_WalksForward verifies the probe finds a newly
// published year past the compiled-in LatestVIIRSYear constant — the
// whole point of not hardcoding it.
func TestNewestVIIRSYear_WalksForward(t *testing.T) {
	t.Cleanup(remote.Capture(remote.VIIRSAnnual).Restore)

	// Upstream has published two years beyond the constant.
	viirsYearServer(t, map[int]bool{
		atlas.LatestVIIRSYear + 1: true,
		atlas.LatestVIIRSYear + 2: true,
	})

	got, err := atlas.NewestVIIRSYear(context.Background())
	if err != nil {
		t.Fatalf("NewestVIIRSYear: %v", err)
	}

	if want := atlas.LatestVIIRSYear + 2; got != want {
		t.Errorf("NewestVIIRSYear = %d, want %d", got, want)
	}
}

// TestNewestVIIRSYear_NoNewerYear verifies the common case — the constant
// is current — costs exactly one probe and returns it unchanged.
func TestNewestVIIRSYear_NoNewerYear(t *testing.T) {
	t.Cleanup(remote.Capture(remote.VIIRSAnnual).Restore)

	probed := viirsYearServer(t, map[int]bool{}) // nothing newer published

	got, err := atlas.NewestVIIRSYear(context.Background())
	if err != nil {
		t.Fatalf("NewestVIIRSYear: %v", err)
	}

	if got != atlas.LatestVIIRSYear {
		t.Errorf("NewestVIIRSYear = %d, want %d", got, atlas.LatestVIIRSYear)
	}

	if len(*probed) != 1 {
		t.Errorf("probed %v (%d requests), want exactly 1 — the walk must stop at the first gap", *probed, len(*probed))
	}
}

// TestNewestVIIRSYear_DegradesWhenUnreachable verifies a probe failure is
// not fatal: the caller still gets a usable year (never worse than the
// compiled-in constant) alongside the error.
func TestNewestVIIRSYear_DegradesWhenUnreachable(t *testing.T) {
	t.Cleanup(remote.Capture(remote.VIIRSAnnual).Restore)

	remote.SetOffline(true)

	got, err := atlas.NewestVIIRSYear(context.Background())
	if err == nil {
		t.Error("expected an error while offline")
	}

	if got != atlas.LatestVIIRSYear {
		t.Errorf("year = %d, want the known-good %d even on failure", got, atlas.LatestVIIRSYear)
	}
}

// TestEnsureVIIRSAnnual_AcceptsYearPastConstant is a regression test for a
// real break: EnsureVIIRSAnnual used to reject any year above the
// compiled-in LatestVIIRSYear, which silently defeated NewestVIIRSYear —
// the probe would discover a newly-published year and the downloader would
// then refuse to fetch it. A future year must get past the range guard and
// fail (if at all) on the download itself, never on ErrVIIRSYearOutOfRange.
func TestEnsureVIIRSAnnual_AcceptsYearPastConstant(t *testing.T) {
	t.Cleanup(remote.Capture(remote.VIIRSAnnual).Restore)
	remote.SetDataDir(gofs.File(t.TempDir()))

	_, err := atlas.EnsureVIIRSAnnual(context.Background(), atlas.LatestVIIRSYear+1)
	if errors.Is(err, atlas.ErrVIIRSYearOutOfRange) {
		t.Fatalf("year %d rejected by the range guard — this is what breaks NewestVIIRSYear: %v",
			atlas.LatestVIIRSYear+1, err)
	}

	// It should instead stop at the consent gate, proving it got through
	// validation and reached the real download path.
	if !errors.Is(err, remote.ErrDownloadDenied) {
		t.Errorf("expected the call to reach the download-consent gate, got %v", err)
	}

	// A year before the mission began is still rejected outright.
	if _, err := atlas.EnsureVIIRSAnnual(context.Background(), atlas.EarliestVIIRSYear-1); !errors.Is(err, atlas.ErrVIIRSYearOutOfRange) {
		t.Errorf("year %d: got %v, want ErrVIIRSYearOutOfRange", atlas.EarliestVIIRSYear-1, err)
	}
}

// TestLayer_String verifies every named Layer renders a stable, non-
// numeric label, and an out-of-range value falls back to a numeric form.
func TestLayer_String(t *testing.T) {
	t.Parallel()

	cases := map[atlas.Layer]string{
		atlas.LayerAuto:              "auto",
		atlas.LayerWorldAtlas:        "world-atlas",
		atlas.LayerVIIRS:             "viirs",
		atlas.LayerLightPollutionMap: "light-pollution-map",
		atlas.LayerBortle:            "bortle",
		atlas.LayerScalar:            "scalar",
		atlas.Layer(99):              "Layer(99)",
	}

	for layer, want := range cases {
		if got := layer.String(); got != want {
			t.Errorf("Layer(%d).String() = %q, want %q", int(layer), got, want)
		}
	}
}
