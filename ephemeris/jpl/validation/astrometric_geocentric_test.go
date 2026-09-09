//go:build network

package jpl_test

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/internal/metrology"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
)

// TestAstrometricAgreesWithHorizonsGeocentric compares astrogo's astrometric
// place against the column Horizons publishes as astrometric, with the Earth's
// orientation taken out of the question entirely.
//
// # Why this exists
//
// Every other comparison in this package ends at observed alt/az. That is the
// right thing to validate — it is what a user asks for — but it folds the
// whole pipeline into one residual: ephemeris, light time, aberration,
// precession-nutation, Earth rotation angle, polar motion, the site vector and
// refraction. A topocentric run measured a cross-track signed mean of about
// -0.52 arcseconds, and a single number at the end of that chain cannot say
// which link it came from.
//
// A geocentric astrometric comparison removes most of the chain by
// construction. CENTER='500@399' is the geocentre, so there is no site vector
// and no Earth rotation; quantity 1 is light-time corrected but carries no
// aberration and no deflection. What is left depends on the ephemeris, the
// light-time solution, and the frame — and on no Earth Orientation Parameter
// at all.
//
// So a bias that survives here is upstream, in the ephemeris or the light-time
// iteration. A bias that vanishes here is downstream, in the parts this
// comparison deliberately excludes. That is the whole purpose: to split one
// number into two answers.
//
// # What is asserted
//
// Both the spread and the *signed* means. A systematic offset is the thing
// being hunted, and it hides completely in an unsigned statistic: a hundred
// residuals of +0.5 arcseconds and a hundred of -0.5 have the same mean
// absolute error as two hundred at zero.
func TestAstrometricAgreesWithHorizonsGeocentric(t *testing.T) {
	testutil.RequireReachable(t, "ssd.jpl.nasa.gov:443")

	// DE441, the ephemeris Horizons itself reports for these queries, rather
	// than the de440 its neighbours in this package use. Part 2 is the half
	// covering the modern era; part 1 is the ancient one and is not needed
	// for 2026.
	provider, err := jpl.NewProvider(context.Background(), core.Planets, "de441_part-2")
	if err != nil {
		t.Fatalf("de441_part-2 provider: %v", err)
	}

	defer func() { _ = provider.Close() }()

	// Bodies spanning the geometry: the Sun (always near-perpendicular to the
	// Earth's velocity), an inner planet, and outer ones where the light time
	// is tens of minutes rather than seconds.
	//
	// Jupiter and Saturn are asked for as '5' and '6' — the system
	// barycentres — not '599' and '699', the body centres. That is what a DE
	// planetary kernel contains for the giant planets: their satellite
	// systems live in separate kernels, so a planetary-kernel provider
	// necessarily returns the barycentre.
	//
	// It is not a detail. Asking Horizons for the body centres instead gives
	// Uranus p50 0.0497", Jupiter 0.0324", Saturn 0.0288" and Neptune
	// 0.0093", while Sun, Venus and Mars sit at exactly zero — the offset
	// between a giant planet and the barycentre it shares with its moons,
	// about 100 km at Jupiter and 200 km at Saturn. Comparing against the
	// wrong centre reads as an astrogo error and is not one (#253).
	bodies := []struct {
		command string
		name    string
		id      core.ID
	}{
		{"10", "Sun", core.Sun},
		{"299", "Venus", core.Venus},
		{"499", "Mars", core.Mars},
		{"5", "Jupiter", core.Jupiter},
		{"6", "Saturn", core.Saturn},
	}

	// The reference records what it shares with astrogo as well as what it
	// is, and here it shares everything: Horizons reports {source: DE441} and
	// this provider reads DE441. That is deliberate. The ephemeris is not
	// what is being tested — astrogo's light-time iteration, frame handling
	// and interpolation are — so holding it identical on both sides removes
	// it as a variable instead of leaving a small unknown in the residual.
	//
	// It also makes the row's honesty non-negotiable: with the same
	// ephemeris on both sides this is *no* evidence whatever about DE, and
	// SharedAncestor is the field that makes a generated report say so rather
	// than leaving a reader to infer it from the version string.
	//
	// Measured, the choice costs nothing either way: the same comparison
	// against a de440 provider gives p50 1.492e-06 and max 3.137e-06
	// arcseconds against DE441's 1.496e-06 and 3.147e-06 — the two agree to
	// about a hundredth of a microarcsecond over 2026, which is what fitting
	// them together over the modern era is supposed to achieve.
	reference := metrology.Reference{
		Kind:           metrology.KindHorizons,
		Name:           "JPL Horizons",
		Version:        "quantity 1 (astrometric), DE441",
		Source:         "https://ssd.jpl.nasa.gov/api/horizons.api",
		Dataset:        "CENTER='500@399', ANG_FORMAT='DEG', EXTRA_PREC='YES'",
		SharedAncestor: "JPL DE441 — both sides read the same ephemeris, by design",
	}

	suite := metrology.NewSuite("ephemeris.astrometric.geocentric", reference,
		metrology.MustContract(0.01, "arcsec",
			"far above the measured zero so a DE revision moving the residual by "+
				"milliarcseconds does not fail this, and fifty times below the ~0.5 arcsec "+
				"topocentric bias the comparison exists to locate — which is the only "+
				"property that makes it evidence",
			"measured: 5 bodies x 13 epochs across 2026, max 3.1e-06 arcsec, which is the "+
				"~3.6 microarcsecond precision Horizons prints with EXTRA_PREC"))

	var (
		separations []float64
		dRACosDec   []float64
		dDec        []float64
	)

	for _, body := range bodies {
		rows, err := fetchGeocentricSeries(body.command, body.name, "1",
			"2026-01-01", "2026-12-27", "30d")
		if err != nil {
			testutil.SkipOnUpstreamFailure(t, err)
			t.Fatalf("%s: fetching the geocentric astrometric series: %v", body.name, err)
		}

		if len(rows) == 0 {
			t.Fatalf("%s: Horizons returned no rows", body.name)
		}

		var bodySeparations []float64

		for _, row := range rows {
			epoch := time.FromJD(row.jdUT, time.UTC)

			state, err := eph.AstrometricState(provider, body.id, epoch)
			if err != nil {
				t.Fatalf("%s: AstrometricState: %v", body.name, err)
			}

			var got coord.Astrometric

			got.FromUnitVector(state.Pos.Unit())

			gotRA := got.RA().Degrees()
			gotDec := got.Dec().Degrees()

			// cos(dec) turns a right-ascension difference into an angle on
			// the sky. Without it a residual near the pole reads as large
			// when it is not.
			dRA := wrapDegrees(gotRA-row.raDeg) * math.Cos(gotDec*math.Pi/180)
			dD := gotDec - row.decDeg

			sep := math.Hypot(dRA, dD) * 3600

			suite.Add(metrology.Sample{
				Error:   sep,
				Label:   body.name,
				Context: fmt.Sprintf("JD %.5f UT, geocentric astrometric", row.jdUT),
			})

			dRACosDec = append(dRACosDec, dRA*3600)
			dDec = append(dDec, dD*3600)
			separations = append(separations, sep)
			bodySeparations = append(bodySeparations, sep)
		}

		// Per body, because the aggregate hides which one carries the tail —
		// and which body it is says what the residual is made of. A fast
		// mover is sensitive to the epoch, a slow one to the ephemeris.
		sort.Float64s(bodySeparations)

		t.Logf("%-8s n=%2d  p50 %7.4f\"  max %7.4f\"",
			body.name, len(bodySeparations),
			bodySeparations[len(bodySeparations)/2],
			bodySeparations[len(bodySeparations)-1])
	}

	if len(separations) < 50 {
		t.Fatalf("only %d comparison points; the matrix is too small to say anything",
			len(separations))
	}

	report := func(label string, xs []float64) (mean float64) {
		for _, x := range xs {
			mean += x
		}

		mean /= float64(len(xs))

		sorted := append([]float64(nil), xs...)
		sort.Float64s(sorted)

		t.Logf("%-14s signed mean %+8.4f\"  p50 %+8.4f\"  p95 %+8.4f\"  min %+8.4f\"  max %+8.4f\"",
			label, mean, sorted[len(sorted)/2], sorted[len(sorted)*95/100], sorted[0], sorted[len(sorted)-1])

		return mean
	}

	meanRA := report("dRA*cos(dec)", dRACosDec)
	meanDec := report("dDec", dDec)

	sorted := append([]float64(nil), separations...)
	sort.Float64s(sorted)

	p50, p95, worst := sorted[len(sorted)/2], sorted[len(sorted)*95/100], sorted[len(sorted)-1]

	t.Logf("separation      p50 %8.4f\"  p95 %8.4f\"  max %8.4f\"  (n=%d)",
		p50, p95, worst, len(separations))

	// The contract is checked and the row emitted by the framework, so this
	// appears in the generated accuracy table with its reference, its shared
	// ancestry and its measured distribution rather than as a claim typed
	// into a document by hand.
	suite.Report(t)

	// The signed means are the point of the whole exercise.
	//
	// The topocentric comparison in this package measures a cross-track
	// signed mean near -0.52 arcseconds. Here, with the ephemeris and the
	// light-time solution isolated and no Earth orientation in the path,
	// both signed means are zero to the printed precision. That is the
	// result: the bias is not upstream. It is somewhere in what this test
	// deliberately excludes — the site vector, Earth rotation, polar motion,
	// or the aberration and refraction applied downstream.
	const maxSignedBiasArcsec = 0.005

	if math.Abs(meanRA) > maxSignedBiasArcsec || math.Abs(meanDec) > maxSignedBiasArcsec {
		t.Errorf("signed bias dRA*cos(dec) %+.4f\", dDec %+.4f\", want both within %.3f\"; "+
			"a systematic offset with no Earth orientation in the path points at the "+
			"ephemeris or the light-time solution, which were previously ruled out",
			meanRA, meanDec, maxSignedBiasArcsec)
	}
}

// wrapDegrees folds a difference of angles into (-180, 180].
//
// Right ascension wraps, so a target either side of 0h differs by ~360
// degrees in raw subtraction and by almost nothing on the sky.
func wrapDegrees(d float64) float64 {
	for d > 180 {
		d -= 360
	}

	for d <= -180 {
		d += 360
	}

	return d
}

// astrometricRow is one Horizons astrometric position: the epoch, and the
// right ascension and declination in degrees.
type astrometricRow struct {
	jdUT   float64
	raDeg  float64
	decDeg float64
}

// fetchGeocentricSeries pulls astrometric positions from a Horizons OBSERVER
// table centred on the geocentre.
//
// CENTER='500@399' rather than 'coord@399': no site, so no site vector and no
// Earth rotation enter the comparison.
//
// ANG_FORMAT='DEG' with EXTRA_PREC='YES', and its own parser rather than
// parseObserverRow, because the default sexagesimal table is quantised far
// too coarsely to measure anything here. Horizons prints it to 0.01 seconds
// of right ascension and 0.1 arcseconds of declination — 0.15 and 0.1
// arcseconds respectively. A first run against that table returned a
// separation of p50 0.045", max 0.106", which is precisely what rounding at
// those steps produces and says nothing whatever about astrogo.
//
// With extra precision the columns carry nine decimal places, about 3.6
// microarcseconds, which is below anything this comparison could care about.
// The lesson is worth keeping: agreement measured against a rounded reference
// is a measurement of the rounding.
func fetchGeocentricSeries(command, bodyName, quantity, startStr, stopStr, stepStr string) ([]astrometricRow, error) {
	_ = bodyName

	params := url.Values{}
	params.Add("format", "text")
	params.Add("COMMAND", "'"+command+"'")
	params.Add("CENTER", "'500@399'")
	params.Add("MAKE_EPHEM", "'YES'")
	params.Add("EPHEM_TYPE", "'OBSERVER'")
	params.Add("START_TIME", fmt.Sprintf("'%s'", startStr))
	params.Add("STOP_TIME", fmt.Sprintf("'%s'", stopStr))
	params.Add("STEP_SIZE", fmt.Sprintf("'%s'", stepStr))
	params.Add("QUANTITIES", "'"+quantity+"'")
	params.Add("APPARENT", "'AIRLESS'")
	params.Add("CAL_FORMAT", "'JD'")
	params.Add("ANG_FORMAT", "'DEG'")
	params.Add("EXTRA_PREC", "'YES'")
	params.Add("CSV_FORMAT", "'YES'")
	params.Add("OBJ_DATA", "'NO'")

	encodedQuery := strings.ReplaceAll(params.Encode(), "+", "%20")
	reqURL := "https://ssd.jpl.nasa.gov/api/horizons.api?" + encodedQuery

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building the request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("querying Horizons: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	bodyBytes, _ := io.ReadAll(resp.Body)
	responseStr := string(bodyBytes)

	if horizonsUnavailable(responseStr) {
		return nil, errHorizonsUnavailable
	}

	soeIdx := strings.Index(responseStr, "$$SOE")

	eoeIdx := strings.Index(responseStr, "$$EOE")
	if soeIdx == -1 || eoeIdx == -1 {
		return nil, errNoEphemerisData
	}

	var rows []astrometricRow

	for line := range strings.SplitSeq(strings.TrimSpace(responseStr[soeIdx+6:eoeIdx]), "\n") {
		cols := strings.Split(line, ",")

		// Read from the end, like parseObserverRow and for the same reason:
		// Horizons emits a varying number of empty presence-flag columns
		// after the date, and a fixed index would silently read one of them.
		if len(cols) < 4 {
			continue
		}

		jd, err := strconv.ParseFloat(strings.TrimSpace(cols[0]), 64)
		if err != nil {
			continue
		}

		ra, err := strconv.ParseFloat(strings.TrimSpace(cols[len(cols)-3]), 64)
		if err != nil {
			continue
		}

		dec, err := strconv.ParseFloat(strings.TrimSpace(cols[len(cols)-2]), 64)
		if err != nil {
			continue
		}

		rows = append(rows, astrometricRow{jdUT: jd, raDeg: ra, decDeg: dec})
	}

	if len(rows) == 0 {
		return nil, errNoObserverRows
	}

	return rows, nil
}
