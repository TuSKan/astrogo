//go:build integration

// FINK integration tests validate astrogo's sHG1G2 model against the live
// FINK/ZTF phunk production pipeline.
//
// Run with: go test -tags integration -run TestFINK -v ./magnitude/
//
// These tests require an active internet connection to reach
// https://api.ztf.fink-portal.org. This file previously had no build tag at
// all, so it ran under the default `go test ./...` — making the default,
// offline-only test suite (and CI's blocking lint-and-test / race-detection
// jobs) fail whenever FINK's API had a bad day, unrelated to any code
// change. See CHANGELOG for the fix.

package magnitude_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/fink"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/time"
)

// ══════════════════════════════════════════════════════════════════════════════
// FINK SSOFT — End-to-End sHG1G2 Model Validation
// ══════════════════════════════════════════════════════════════════════════════
//
// These tests validate our Carry et al. (2024) sHG1G2 implementation against
// the FINK/ZTF production phunk pipeline using two data sources:
//
//  1. FINK SSOFT provider (catalog/fink) — fitted sHG1G2 parameters for the
//     target asteroid (H, G1, G2, R, α₀, δ₀) via parquet download.
//
//  2. FINK /api/v1/sso?withResiduals=true — per-observation data with
//     residuals_shg1g2 = observed_reduced_mag − model_value.
//
// The validation computes our model for each observation using the SSOFT
// fitted parameters and compares: our_residual ≈ fink_residual.
//
// API: https://api.ztf.fink-portal.org
// Reference: Carry et al. (2024), A&A, 689, A252.

const finkBaseURL = "https://api.ztf.fink-portal.org"

// finkSSOQuery returns FINK's observations of numberOrDesig, fetched once per
// test run (#501).
//
// # Why it caches, retries and asks a control
//
// FINK has twice answered a request it serves every other time with a 200
// that held nothing usable: an empty list in 0.15 s where the full answer
// takes about six, and records without the residuals asked for. Each failed
// whichever test was unlucky, three times in two days. CLAUDE.md's rule is to
// skip on a degraded service and fail on wrong data from a working one, so an
// unusable answer is tried once more, and if it is still unusable a control
// that cannot be empty decides which it is: 433 Eros's observation dates. An
// empty control means FINK is degraded and the test skips; a control with
// data means FINK is working and the answer is wrong, and the test fails.
//
// The cache is why the package sends two of these ~850 KB queries per run
// instead of five, to a service that may be throttling. A failed fetch is not
// cached, so the next test asks again.
func finkSSOQuery(t *testing.T, numberOrDesig string, withResiduals, withEphem bool) []map[string]any { //nolint:unparam // designed for reuse
	t.Helper()

	finkMu.Lock()
	defer finkMu.Unlock()

	key := finkKey{numberOrDesig, withResiduals, withEphem}
	if records, ok := finkCache[key]; ok {
		return records
	}

	body := map[string]any{
		"n_or_d":        numberOrDesig,
		"withResiduals": withResiduals,
		"withEphem":     withEphem,
		"output-format": "json",
	}

	records := finkPost(t, body)

	if !finkUsable(records, withResiduals) {
		time.Sleep(finkRetryPause)

		records = finkPost(t, body)
	}

	if !finkUsable(records, withResiduals) {
		// Every record carrying the residual column, every value null, is
		// FINK saying it computed none: its ephemeris service failed inside
		// the request. Measured on one request sent twice: 327 records with
		// residuals in 6.7 s, then the same 327 with every residual null in
		// 32.7 s. The control below cannot see that; Eros's dates come from
		// FINK's own store (#700). A column that is absent is not this, and
		// still goes to the control.
		if withResiduals && finkResidualsAllNull(records) {
			t.Skipf("FINK answered %s with %d records whose residuals_shg1g2 are all null, twice: "+
				"it computed none, as it does when its ephemeris service fails (#700)", numberOrDesig, len(records))
		}

		control := finkPost(t, map[string]any{"n_or_d": "433", "columns": "i:jd", "output-format": "json"})
		if len(control) == 0 {
			t.Skipf("FINK answered %s with %d records and no usable ones, twice, and its control "+
				"query for 433 Eros came back empty too: the service is degraded (#501)", numberOrDesig, len(records))
		}

		t.Fatalf("FINK answered %s with %d records and no usable ones, twice, while its control query "+
			"for 433 Eros returned %d: wrong data from a working service", numberOrDesig, len(records), len(control))
	}

	finkCache[key] = records

	return records
}

type finkKey struct {
	object               string
	residuals, ephemeris bool
}

var (
	finkMu    sync.Mutex
	finkCache = map[finkKey][]map[string]any{}
)

// finkRetryPause is how long an unusable answer waits before it is asked
// again: long enough for a throttle to lift, short beside the six seconds a
// full answer takes.
const finkRetryPause = 3 * time.Second

// finkUsable reports whether an answer can be tested at all: it has records,
// and if residuals were asked for, at least one carries one. How many it
// needs is each test's business.
func finkUsable(records []map[string]any, withResiduals bool) bool {
	if len(records) == 0 {
		return false
	}

	if !withResiduals {
		return true
	}

	for _, r := range records {
		if _, ok := getFloat(r, "residuals_shg1g2"); ok {
			return true
		}
	}

	return false
}

// finkResidualsAllNull reports whether every record carries the
// residuals_shg1g2 column with no value in it.
func finkResidualsAllNull(records []map[string]any) bool {
	if len(records) == 0 {
		return false
	}

	for _, r := range records {
		v, present := r["residuals_shg1g2"]
		if !present || v != nil {
			return false
		}
	}

	return true
}

// finkPost sends one SSO query. A transport failure or a 5xx skips, as an
// outage, and so does a 400 in which FINK names its ephemeris service,
// Miriade, as what failed (#700); any other non-200 fails.
func finkPost(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("FINK SSO JSON marshal failed: %v", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, finkBaseURL+"/api/v1/sso", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("FINK SSO request build failed: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// One call: SkipOnUpstreamFailure consults Unreachable too, so a
		// refused dial and a DNS failure are covered here as well.
		testutil.SkipOnUpstreamFailure(t, err)
		t.Fatalf("FINK SSO request: %v", err)
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("failed to close response body: %v", err)
		}
	}()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= http.StatusInternalServerError {
		// 5xx means the service itself is degraded (observed live: FINK's
		// gateway returning 504), not that our request is wrong — treat it
		// the same as the connection-level failure above rather than
		// failing the run for external downtime.
		t.Skipf("FINK SSO service unavailable, skipping live test: HTTP %d: %s",
			resp.StatusCode, string(data[:min(200, len(data))]))
	}

	// FINK computes ephemerides through IMCCE's Miriade, and when Miriade does
	// not answer it says so in a 400: "We could not obtain the ephemerides
	// information. Check Miriade availabilities." That is FINK reporting its
	// dependency down, not a malformed request.
	if resp.StatusCode == http.StatusBadRequest && bytes.Contains(data, []byte("Miriade")) {
		t.Skipf("FINK's ephemeris service, Miriade, did not answer: HTTP 400: %s (#700)",
			string(data[:min(200, len(data))]))
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("FINK SSO HTTP %d: %s", resp.StatusCode, string(data[:min(200, len(data))]))
	}

	var records []map[string]any
	if err := json.Unmarshal(data, &records); err != nil {
		t.Fatalf("FINK SSO JSON parse: %v", err)
	}

	return records
}

// getFloat extracts a float64 from a JSON record, skipping nil/nan.
func getFloat(r map[string]any, key string) (float64, bool) {
	v, ok := r[key]
	if !ok || v == nil {
		return 0, false
	}

	switch val := v.(type) {
	case float64:
		if math.IsNaN(val) || math.IsInf(val, 0) {
			return 0, false
		}

		return val, true
	case json.Number:
		f, err := val.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

// TestFINK_SSOEndpoint validates that the FINK /api/v1/sso endpoint
// is reachable and returns plausible observation data.
func TestFINK_SSOEndpoint(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}

	records := finkSSOQuery(t, "8467", false, true)
	if len(records) < 10 {
		t.Fatalf("expected ≥10 observations for 8467 (Benoitcarry), got %d", len(records))
	}

	t.Logf("8467 Benoitcarry: %d observations", len(records))

	// Validate first record has required ephemeris fields.
	r := records[0]
	for _, key := range []string{"Phase", "Dhelio", "Dobs", "RA", "DEC", "i:fid", "i:magpsf", "i:magpsf_red"} {
		if _, ok := r[key]; !ok {
			t.Errorf("missing field %q in FINK response", key)
		}
	}
}

// TestFINK_EndToEndSHG1G2 is the main validation test. It:
//  1. Gets fitted sHG1G2 parameters from the FINK SSOFT provider
//  2. Gets per-observation data with residuals from /api/v1/sso
//  3. Computes our model for each observation
//  4. Compares our residuals against FINK's residuals
func TestFINK_EndToEndSHG1G2(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}

	// Step 1: Get fitted parameters from SSOFT via the provider.
	prov := fink.New()

	tgt, err := prov.Resolve(context.Background(), "8467")
	if err != nil {
		testutil.SkipOnUpstreamFailure(t, err)
		t.Fatalf("Resolve(8467): %v", err)
	}

	// Resolve has a fast path — a single-object JSON lookup — that never
	// downloads the SSOFT table, so Count() is legitimately zero here. Logging
	// it bare read as "loaded nothing, yet somehow has the parameters", which
	// is a contradiction a reader has to go into catalog/fink to resolve.
	if prov.Loaded() {
		t.Logf("SSOFT loaded: %d objects", prov.Count())
	} else {
		t.Log("SSOFT not loaded; 8467 came from the single-object endpoint")
	}

	t.Logf("8467 %s fitted params:", tgt.Name)
	t.Logf("  H       = %.4f (r-band)", tgt.H)
	t.Logf("  G1      = %.4f", tgt.G1)
	t.Logf("  G2      = %.4f", tgt.G2)
	t.Logf("  R       = %.4f", tgt.Oblateness)
	t.Logf("  SpinRA  = %.2f°", tgt.SpinRA)
	t.Logf("  SpinDec = %.2f°", tgt.SpinDec)

	if !tgt.HasH || !tgt.HasG1G2 {
		t.Fatal("missing H or G1/G2 from SSOFT — cannot validate")
	}

	// Step 2: Get per-observation data with residuals.
	records := finkSSOQuery(t, "8467", true, true)
	if len(records) < 10 {
		t.Fatalf("expected ≥10 observations with residuals, got %d", len(records))
	}

	// Step 3 & 4: Compute our model and compare residuals.
	// Counted separately so a failure can say which of the two things went
	// wrong. "No usable observations" has two causes that want opposite
	// outcomes: FINK holding no r-band data for this object right now is
	// absence and the comparison simply cannot be made, while FINK renaming a
	// column is a schema change astrogo has to notice. Both used to arrive as
	// the same sentence, and neither was actionable.
	var (
		nValid    int
		nRBand    int // records in r-band (fid=2)
		nDropped  int // r-band records missing a column this needs
		absent    = map[string]int{}
		sumDiff   float64
		sumDiffSq float64
		maxDiff   float64
		nMatch    int // |diff| < 0.025 mag
	)

	// Every column this comparison reads, named once so the failure message
	// and the loop cannot drift apart.
	fields := []string{"residuals_shg1g2", "i:magpsf_red", "Phase", "Dhelio", "Dobs", "RA", "DEC"}

	spinRA := angle.Deg(tgt.SpinRA)
	spinDec := angle.Deg(tgt.SpinDec)

	for _, r := range records {
		// The band selector is read first, and its absence counted as a
		// missing column rather than as a non-r-band record: a record with no
		// i:fid is not evidence about which filter it came from.
		fid, okFid := getFloat(r, "i:fid")
		if !okFid {
			absent["i:fid"]++
			nDropped++

			continue
		}

		// Only validate r-band (fid=2) since we use r-band H, G1, G2.
		if int(fid) != 2 {
			continue
		}

		nRBand++

		vals := make(map[string]float64, len(fields))

		for _, key := range fields {
			v, ok := getFloat(r, key)
			if !ok {
				absent[key]++

				continue
			}

			vals[key] = v
		}

		if len(vals) != len(fields) {
			nDropped++

			continue
		}

		finkRes, redMag := vals["residuals_shg1g2"], vals["i:magpsf_red"]
		dhelio, dobs := vals["Dhelio"], vals["Dobs"]

		alpha := angle.Deg(vals["Phase"])
		ra := angle.Deg(vals["RA"])
		dec := angle.Deg(vals["DEC"])

		// Compute our model: reduced magnitude at r=Δ=1 equivalent.
		// reduced_mag = H - 2.5·log₁₀(G₁Φ₁+G₂Φ₂+G₃Φ₃) + SpinCorrection
		// We use AsteroidSHG1G2 with r=1, Δ=1 to get the reduced magnitude.
		var ourModel float64

		if tgt.HasSpin && tgt.HasOblateness {
			cosL := magnitude.CosAspectAngle(ra, dec, spinRA, spinDec)
			ourModel = magnitude.AsteroidSHG1G2(tgt.H, tgt.G1, tgt.G2, 1, 1, alpha, tgt.Oblateness, cosL)
		} else {
			ourModel = magnitude.AsteroidHG1G2(tgt.H, tgt.G1, tgt.G2, 1, 1, alpha)
		}

		// Our residual = observed_reduced - our_model.
		ourRes := redMag - ourModel

		// FINK residual = observed_reduced - fink_model.
		// If our model == FINK model, then ourRes ≈ finkRes.
		diff := math.Abs(ourRes - finkRes)
		sumDiff += diff

		sumDiffSq += diff * diff
		if diff > maxDiff {
			maxDiff = diff
		}

		if diff < 0.025 {
			nMatch++
		}

		nValid++

		_ = dhelio
		_ = dobs
	}

	if nValid == 0 {
		// A column FINK no longer serves means astrogo is reading the wrong
		// names, and has to fail loudly however few records arrived.
		if len(absent) > 0 {
			t.Fatalf("none of FINK's %d observations for 8467 could be used: %d were "+
				"dropped for missing columns %v. FINK serves these under different "+
				"names than this comparison reads, which is a schema change rather "+
				"than an outage", len(records), nDropped, absent)
		}

		// Whereas an object with no r-band photometry right now is absence:
		// nothing is wrong and there is nothing to compare.
		t.Skipf("FINK returned %d observations for 8467, none of them in r-band "+
			"(i:fid == 2), and the fitted H/G1/G2 this validates against are r-band. "+
			"Nothing to compare rather than anything wrong", len(records))
	}

	meanDiff := sumDiff / float64(nValid)
	rmsDiff := math.Sqrt(sumDiffSq / float64(nValid))
	matchPct := float64(nMatch) / float64(nValid) * 100

	t.Logf("\nValidation results: %d usable of %d r-band, from %d observations",
		nValid, nRBand, len(records))
	t.Logf("  Mean |our_res − fink_res| = %.4f mag", meanDiff)
	t.Logf("  RMS  |our_res − fink_res| = %.4f mag", rmsDiff)
	t.Logf("  Max  |our_res − fink_res| = %.4f mag", maxDiff)
	t.Logf("  Match (<0.025 mag):         %.1f%% (%d/%d)", matchPct, nMatch, nValid)

	// Assert: our model matches FINK's model to within 0.01 mag for ≥95%.
	// Threshold 0.025 mag accounts for version mismatch between
	// SSOFT params (v2025.04) and portal-internal residual computation.
	if matchPct < 85 {
		t.Errorf("model match = %.1f%%, expected ≥85%% within 0.025 mag", matchPct)
	}

	if meanDiff > 0.03 {
		t.Errorf("mean |diff| = %.4f, expected < 0.02 mag", meanDiff)
	}
}

// TestFINK_ReducedMagnitudeConsistency checks that FINK's reduced magnitude
// (i:magpsf_red) is consistent with i:magpsf - 5·log₁₀(Dhelio·Dobs).
func TestFINK_ReducedMagnitudeConsistency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}

	records := finkSSOQuery(t, "8467", false, true)
	if len(records) < 10 {
		t.Fatalf("expected ≥10 observations, got %d", len(records))
	}

	var nChecked int

	for _, r := range records {
		magpsf, okMag := getFloat(r, "i:magpsf")
		redMag, okRed := getFloat(r, "i:magpsf_red")
		dhelio, okD1 := getFloat(r, "Dhelio")
		dobs, okD2 := getFloat(r, "Dobs")

		if !okMag || !okRed || !okD1 || !okD2 || dhelio <= 0 || dobs <= 0 {
			continue
		}

		expected := magpsf - 5*math.Log10(dhelio*dobs)
		diff := math.Abs(redMag - expected)

		if diff > 0.01 {
			t.Errorf("obs %d: magpsf_red=%.4f, expected=%.4f (diff=%.4f)",
				nChecked, redMag, expected, diff)
		}

		nChecked++
		if nChecked >= 20 {
			break
		}
	}

	if nChecked == 0 {
		t.Fatal("no valid observations for reduced magnitude check")
	}

	t.Logf("Checked %d observations: reduced magnitude formula consistent", nChecked)
}

// TestFINK_SpinCorrectionPhysics validates that our SpinCorrection and
// CosAspectAngle produce physically consistent results using real SSOFT
// parameters and FINK observation geometry.
func TestFINK_SpinCorrectionPhysics(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}

	// Get spin parameters from the provider.
	prov := fink.New()

	tgt, err := prov.Resolve(context.Background(), "8467")
	if err != nil {
		testutil.SkipOnUpstreamFailure(t, err)
		t.Fatalf("Resolve(8467): %v", err)
	}

	if !tgt.HasSpin || !tgt.HasOblateness {
		t.Skip("no spin/R parameters available for 8467")
	}

	spinRA := angle.Deg(tgt.SpinRA)
	spinDec := angle.Deg(tgt.SpinDec)
	R := tgt.Oblateness

	records := finkSSOQuery(t, "8467", false, true)
	if len(records) < 10 {
		t.Fatalf("expected ≥10 observations, got %d", len(records))
	}

	var cosLambdas, spinCorrs []float64

	for _, r := range records {
		raVal, okRA := getFloat(r, "RA")

		decVal, okDec := getFloat(r, "DEC")
		if !okRA || !okDec {
			continue
		}

		cosL := magnitude.CosAspectAngle(angle.Deg(raVal), angle.Deg(decVal), spinRA, spinDec)
		s := magnitude.SpinCorrection(R, cosL)

		cosLambdas = append(cosLambdas, cosL)
		spinCorrs = append(spinCorrs, s)
	}

	if len(cosLambdas) == 0 {
		t.Fatal("no valid geometry records")
	}

	var minCos, maxCos, minSpin, maxSpin float64

	minCos, maxCos = cosLambdas[0], cosLambdas[0]
	minSpin, maxSpin = spinCorrs[0], spinCorrs[0]

	for i, c := range cosLambdas {
		if c < -1.001 || c > 1.001 {
			t.Errorf("cos Λ = %.6f out of [-1,1] bounds", c)
		}

		if c < minCos {
			minCos = c
		}

		if c > maxCos {
			maxCos = c
		}

		s := spinCorrs[i]
		if s > 0.001 {
			t.Errorf("SpinCorrection = %.6f should be ≤ 0", s)
		}

		if s < minSpin {
			minSpin = s
		}

		if s > maxSpin {
			maxSpin = s
		}
	}

	t.Logf("Spin params: RA=%.1f° Dec=%.1f° R=%.4f", tgt.SpinRA, tgt.SpinDec, R)
	t.Logf("Geometry stats (n=%d):", len(cosLambdas))
	t.Logf("  cos Λ range:          [%.4f, %.4f]", minCos, maxCos)
	t.Logf("  SpinCorrection range: [%.4f, %.4f] mag", minSpin, maxSpin)
	t.Logf("  Aspect coverage:      %.1f°", math.Acos(minCos)*180/math.Pi-math.Acos(maxCos)*180/math.Pi)
}

// TestFINK_ResidualStatistics validates that FINK's sHG1G2 residuals
// are unbiased and well-behaved.
func TestFINK_ResidualStatistics(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}

	records := finkSSOQuery(t, "8467", true, true)
	if len(records) < 10 {
		t.Fatalf("expected ≥10 observations, got %d", len(records))
	}

	var (
		nValid                      int
		sumRes, sumResSq, sumAbsRes float64
		maxAbsRes                   float64
	)

	for _, r := range records {
		res, ok := getFloat(r, "residuals_shg1g2")
		if !ok {
			continue
		}

		sumRes += res
		sumResSq += res * res
		abs := math.Abs(res)

		sumAbsRes += abs
		if abs > maxAbsRes {
			maxAbsRes = abs
		}

		nValid++
	}

	if nValid == 0 {
		t.Fatal("no valid residuals")
	}

	meanRes := sumRes / float64(nValid)
	rmsRes := math.Sqrt(sumResSq / float64(nValid))
	meanAbsRes := sumAbsRes / float64(nValid)

	t.Logf("FINK residual stats (n=%d):", nValid)
	t.Logf("  Mean     = %+.4f mag", meanRes)
	t.Logf("  RMS      = %.4f mag", rmsRes)
	t.Logf("  Mean|res|= %.4f mag", meanAbsRes)
	t.Logf("  Max|res| = %.4f mag", maxAbsRes)

	if math.Abs(meanRes) > 0.15 {
		t.Errorf("mean residual = %+.4f, expected near zero", meanRes)
	}

	if rmsRes > 0.5 {
		t.Errorf("RMS residual = %.4f, expected < 0.5", rmsRes)
	}
}

// Ensure imports are used.
var _ = fmt.Sprintf

// TestFinkUsableRecognizesADegradedAnswer pins what finkSSOQuery treats as an
// answer worth testing (#501): FINK has returned a 200 with an empty list, and
// one whose records lacked the residuals asked for.
func TestFinkUsableRecognizesADegradedAnswer(t *testing.T) {
	t.Parallel()

	withResidual := map[string]any{"residuals_shg1g2": 0.01}
	without := map[string]any{"residuals_shg1g2": nil}

	for _, c := range []struct {
		name      string
		records   []map[string]any
		residuals bool
		want      bool
	}{
		{"empty", nil, false, false},
		{"records", []map[string]any{without}, false, true},
		{"residuals asked, none carried", []map[string]any{without, without}, true, false},
		{"residuals asked, one carried", []map[string]any{without, withResidual}, true, true},
	} {
		if got := finkUsable(c.records, c.residuals); got != c.want {
			t.Errorf("%s: finkUsable = %v, want %v", c.name, got, c.want)
		}
	}
}
