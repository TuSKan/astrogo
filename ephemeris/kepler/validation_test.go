//go:build network

package kepler_test

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/ephemeris/kepler"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// Static errors for the Horizons response parser, so a caller can tell a
// changed output format from a network failure with errors.Is.
var (
	errNoDataBlock       = errors.New("horizons: no $$SOE/$$EOE data block in the response")
	errNoElementRows     = errors.New("horizons: no element rows returned")
	errNoVectorRows      = errors.New("horizons: no vector rows returned")
	errUnexpectedColumns = errors.New("horizons: unexpected column count")
)

// requireHorizons skips the test when the JPL Horizons API is
// unreachable — a reachability failure must never fail CI outright,
// matching ephemeris/jpl/validation's own network test policy.
func requireHorizons(t *testing.T) {
	t.Helper()

	testutil.RequireReachable(t, "ssd.jpl.nasa.gov:443")
}

// horizonsStatusError is an HTTP status carried out of horizonsGet as an error, so
// that [testutil.SkipOnUpstreamFailure] can recognize Horizons answering 503
// or 429.
//
// This fetch uses net/http directly rather than remote.Client, so nothing
// upstream of it produces a *remote.HTTPError. Without this, an overloaded
// Horizons returns an HTML error page, the parser reports errNoDataBlock, and
// the suite fails for someone else's maintenance window. testutil matches the
// status through an interface rather than a concrete type — exactly so that a
// caller in this position can supply its own.
type horizonsStatusError int

func (s horizonsStatusError) Error() string { return fmt.Sprintf("horizons: http %d", int(s)) }

func (s horizonsStatusError) HTTPStatus() int { return int(s) }

// TestHorizonsStatusIsRecognizedAsAnOutage checks that the type above actually
// reaches the classifier.
//
// It is matched structurally, through an interface testutil declares inline and
// nothing here names, so there is no compile-time link between the two. Spell
// the method HttpStatus and this file still builds, still passes, and silently
// never skips again — which is the failure it was written to prevent.
//
// 404 is in the table for the other half of the contract: a status that means
// astrogo asked for the wrong thing must stay fatal.
func TestHorizonsStatusIsRecognizedAsAnOutage(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		wantSkip bool
	}{
		{"overloaded", http.StatusServiceUnavailable, true},
		{"rate limited", http.StatusTooManyRequests, true},
		{"not found", http.StatusNotFound, false},
	} {
		var skipped bool

		t.Run(tc.name, func(t *testing.T) {
			// Read in a defer because Skipf ends the subtest through
			// runtime.Goexit, so nothing after the call below would run.
			defer func() { skipped = t.Skipped() }()

			testutil.SkipOnUpstreamFailure(t, horizonsStatusError(tc.status))
		})

		if skipped != tc.wantSkip {
			t.Errorf("http %d: skipped = %v, want %v", tc.status, skipped, tc.wantSkip)
		}
	}
}

// horizonsGet issues a GET against the Horizons API and returns the
// $$SOE/$$EOE-delimited data block.
func horizonsGet(params url.Values) (string, error) {
	encoded := strings.ReplaceAll(params.Encode(), "+", "%20")
	reqURL := "https://ssd.jpl.nasa.gov/api/horizons.api?" + encoded

	resp, err := http.Get(reqURL) //nolint:noctx // fixed, trusted host; network-tagged test, not production code
	if err != nil {
		return "", fmt.Errorf("horizons request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Before reading the body: a non-200 body is an error page, and reporting
	// it as a parse failure is what made a Horizons outage look like a defect
	// here.
	if resp.StatusCode != http.StatusOK {
		return "", horizonsStatusError(resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read horizons response: %w", err)
	}

	s := string(body)

	soe := strings.Index(s, "$$SOE")
	eoe := strings.Index(s, "$$EOE")

	if soe == -1 || eoe == -1 {
		return "", fmt.Errorf("%w: %.500s", errNoDataBlock, s)
	}

	return strings.TrimSpace(s[soe+6 : eoe]), nil
}

// fetchHelioElements queries Horizons for designation's real published
// heliocentric osculating elements (EPHEM_TYPE=ELEMENTS, CENTER='@10')
// at atStr, and the exact TDB Julian date they were reported for.
// Verified live against Horizons' real ELEMENTS CSV column layout
// (JDTDB, Calendar Date, EC, QR, IN, OM, W, Tp, N, MA, TA, A, AD, PR;
// km-seconds units by default, since OUT_UNITS is not set) — not
// guessed — and cross-checked against 433 Eros's well-known real
// elements (a~1.458 AU, e~0.223, i~10.83 deg) before being trusted.
// horizonsEpoch renders an instant for a Horizons query that declares
// TIME_TYPE='TDB', as a Julian Date rather than a calendar string.
//
// A calendar string cannot carry its own scale, and formatting one here got
// the scale wrong twice over. Time.Format renders the *UTC* calendar
// representation — 2026-01-01 00:00 TDB comes out as 2025-12-31 23:58:50 —
// and the query then told Horizons to read that as TDB, shifting the request
// by 69.18 seconds. It happened once for the elements and again for the
// vectors they were compared against, so the two disagreed by about 138
// seconds of Eros's motion: 4.1 arcseconds, where the elements and vectors
// themselves agree to 0.04.
//
// The test used to pass, and that is the interesting part. Before the ToGo
// fix in #50, Time.Format reinterpreted any scale as UTC, so a TDB instant
// rendered its TDB calendar fields — exactly what the query needed. Fixing
// ToGo made this test start failing, which is what a test resting on a
// defect does when the defect goes away.
//
// A Julian Date has no calendar and therefore no scale to lose: JD2461041.5
// with TIME_TYPE='TDB' means what it says.
func horizonsEpoch(at time.Time) string {
	return fmt.Sprintf("'JD%.9f'", at.JD())
}

func fetchHelioElements(designation string, at time.Time) (epochJD float64, el kepler.Elements, err error) {
	params := url.Values{}
	params.Add("format", "text")
	params.Add("COMMAND", fmt.Sprintf("'%s'", designation))
	params.Add("CENTER", "'@10'")
	params.Add("MAKE_EPHEM", "'YES'")
	params.Add("EPHEM_TYPE", "'ELEMENTS'")
	params.Add("START_TIME", horizonsEpoch(at))
	params.Add("STOP_TIME", horizonsEpoch(at.Add(unit.Days(1.0/1440))))
	params.Add("STEP_SIZE", "'1m'")
	params.Add("TIME_TYPE", "'TDB'")
	params.Add("CAL_FORMAT", "'JD'")
	params.Add("CSV_FORMAT", "'YES'")
	params.Add("OBJ_DATA", "'NO'")

	block, err := horizonsGet(params)
	if err != nil {
		return 0, kepler.Elements{}, err
	}

	lines := strings.Split(block, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return 0, kepler.Elements{}, fmt.Errorf("%w: %q", errNoElementRows, designation)
	}

	cols := strings.Split(lines[0], ",")
	if len(cols) < 14 {
		return 0, kepler.Elements{}, fmt.Errorf("%w: elements output had %d", errUnexpectedColumns, len(cols))
	}

	f := func(idx int) (float64, error) {
		return strconv.ParseFloat(strings.TrimSpace(cols[idx]), 64)
	}

	jdtdb, err := f(0)
	if err != nil {
		return 0, kepler.Elements{}, fmt.Errorf("parse JDTDB: %w", err)
	}

	ec, err := f(2) // eccentricity
	if err != nil {
		return 0, kepler.Elements{}, fmt.Errorf("parse EC: %w", err)
	}

	incl, err := f(4) // deg
	if err != nil {
		return 0, kepler.Elements{}, fmt.Errorf("parse IN: %w", err)
	}

	node, err := f(5) // deg
	if err != nil {
		return 0, kepler.Elements{}, fmt.Errorf("parse OM: %w", err)
	}

	argp, err := f(6) // deg
	if err != nil {
		return 0, kepler.Elements{}, fmt.Errorf("parse W: %w", err)
	}

	ma, err := f(9) // deg
	if err != nil {
		return 0, kepler.Elements{}, fmt.Errorf("parse MA: %w", err)
	}

	aKm, err := f(11) // km (default KM-S units, OUT_UNITS not set)
	if err != nil {
		return 0, kepler.Elements{}, fmt.Errorf("parse A: %w", err)
	}

	const kmPerAU = 149_597_870.7

	el, err = kepler.NewElements(
		time.FromJDParts(jdtdb, 0, time.TDB), unit.AU((aKm / kmPerAU)), ec,
		angle.Deg(incl), angle.Deg(node), angle.Deg(argp), angle.Deg(ma),
	)
	if err != nil {
		return 0, kepler.Elements{}, fmt.Errorf("build elements: %w", err)
	}

	return jdtdb, el, nil
}

// fetchHelioVector queries Horizons for designation's real heliocentric
// position/velocity (EPHEM_TYPE=VECTORS, CENTER='@10', REF_PLANE='FRAME'
// — the ICRF/J2000 equatorial frame [Elements.StateAt] itself returns,
// OUT_UNITS='AU-D') at.
func fetchHelioVector(designation string, at time.Time) (pos, vel vector.Vec3, err error) {
	params := url.Values{}
	params.Add("format", "text")
	params.Add("COMMAND", fmt.Sprintf("'%s'", designation))
	params.Add("CENTER", "'@10'")
	params.Add("MAKE_EPHEM", "'YES'")
	params.Add("EPHEM_TYPE", "'VECTORS'")
	params.Add("START_TIME", horizonsEpoch(at))
	params.Add("STOP_TIME", horizonsEpoch(at.Add(unit.Days(1.0/1440))))
	params.Add("STEP_SIZE", "'1m'")
	params.Add("TIME_TYPE", "'TDB'")
	params.Add("OUT_UNITS", "'AU-D'")
	params.Add("REF_PLANE", "'FRAME'")
	params.Add("VEC_TABLE", "'2'")
	params.Add("CSV_FORMAT", "'YES'")
	params.Add("OBJ_DATA", "'NO'")

	block, err := horizonsGet(params)
	if err != nil {
		return vector.Zero(), vector.Zero(), err
	}

	lines := strings.Split(block, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return vector.Zero(), vector.Zero(), fmt.Errorf("%w: %q", errNoVectorRows, designation)
	}

	cols := strings.Split(lines[0], ",")
	if len(cols) < 8 {
		return vector.Zero(), vector.Zero(), fmt.Errorf("%w: vector output had %d", errUnexpectedColumns, len(cols))
	}

	f := func(idx int) (float64, error) {
		return strconv.ParseFloat(strings.TrimSpace(cols[idx]), 64)
	}

	x, errX := f(2)
	y, errY := f(3)
	z, errZ := f(4)
	vx, errVX := f(5)
	vy, errVY := f(6)
	vz, errVZ := f(7)

	if errX != nil || errY != nil || errZ != nil || errVX != nil || errVY != nil || errVZ != nil {
		return vector.Zero(), vector.Zero(), fmt.Errorf("parse vector columns: %w", errors.Join(errX, errY, errZ, errVX, errVY, errVZ))
	}

	return vector.V3(x, y, z), vector.V3(vx, vy, vz), nil
}

// angularSeparationArcsec returns the angle between two vectors, in
// arcseconds, via the numerically stable atan2(|cross|, dot) form.
func angularSeparationArcsec(a, b vector.Vec3) float64 {
	cross := a.Cross(b).Norm()
	dot := a.Dot(b)
	rad := math.Atan2(cross, dot)

	return rad * 180 / math.Pi * 3600
}

// TestElements_StateAt_AgainstHorizons_433Eros propagates 433 Eros's
// real published osculating elements (fetched live, not hardcoded) via
// two-body Keplerian motion and compares the result against Horizons'
// own real (perturbed) heliocentric ephemeris over +/-30 days from the
// elements' own epoch. 433 Eros is a well-numbered, well-observed
// near-Earth asteroid with long-stable published elements — a
// reasonable, undramatic choice for this cross-check.
//
// Two-body propagation ignores planetary perturbations by design (see
// the kepler package doc comment), so some real divergence from the
// perturbed ephemeris is expected and grows with |dt|. A live run of
// this exact comparison measured it at 0.000" at dt=0, growing to
// ~0.58" at dt=+/-30d — toleranceArcsec below keeps margin above that
// measured max rather than claiming perturbation-level accuracy this
// package doesn't attempt.
//
// dt=0 is held far tighter, because there is no physics in it: osculating
// elements reproduce the ephemeris exactly at their own epoch, so anything
// left there is a convention. It read 0.04" until #391 — the elements were
// rotated into the equator by the IAU 2006 obliquity, not the IAU 1976 one
// JPL refers them to — and was taken for perturbation.
func TestElements_StateAt_AgainstHorizons_433Eros(t *testing.T) {
	requireHorizons(t)

	const designation = "433"

	epochJD, el, err := fetchHelioElements(designation, time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC))
	testutil.SkipOnUpstreamFailure(t, err)
	testutil.AssertNoError(t, err)

	if a := el.SemiMajorAxis().AU(); a < 1.0 || a > 2.0 {
		t.Fatalf("sanity check failed: 433 Eros semi-major axis = %v AU, expected ~1.458 AU", a)
	}

	epoch := time.FromJDParts(epochJD, 0, time.TDB)

	// 2 arcsec, chosen from real measured data, not picked in advance: a
	// live run of this exact comparison found the two-body/perturbed
	// divergence grows roughly symmetrically from 0.000" at dt=0 to a
	// max of ~0.58" at dt=+/-30d (see the package's two-body-only
	// accuracy caveat above) — this bound keeps real margin above that
	// measured max rather than chasing it exactly.
	//
	// At dt=0 there is nothing to allow for; the elements' printed digits
	// are good to well under a milliarcsecond there.
	const (
		toleranceArcsec     = 2.0
		atEpochToleranceArc = 0.005
	)

	var (
		maxSepArcsec      float64
		compared, outages int
	)

	for _, dtDays := range []float64{-30, -20, -10, -5, 0, 5, 10, 20, 30} {
		at := epoch.Add(unit.Days(dtDays))

		wantPos, _, err := fetchHelioVector(designation, at)
		if err != nil {
			// A Horizons hiccup for one of the nine points costs that
			// point: it has been seen to fail four in a row and answer on
			// an immediate retry. Only an identified outage is skipped;
			// anything else is this test's request or parser, and fails.
			if reason, ok := testutil.UpstreamFailure(err); ok {
				t.Logf("dt=%+.0fd: Horizons did not answer (%s): %v", dtDays, reason, err)

				outages++

				continue
			}

			t.Fatalf("dt=%+.0fd: fetch: %v", dtDays, err)
		}

		compared++

		gotPos, _, err := el.StateAt(at)
		testutil.AssertNoError(t, err)

		sepArcsec := angularSeparationArcsec(gotPos, wantPos)
		t.Logf("dt=%+.0fd sep=%.3f\" (|r|=%.4f AU)", dtDays, sepArcsec, gotPos.Norm())

		if sepArcsec > maxSepArcsec {
			maxSepArcsec = sepArcsec
		}

		if dtDays == 0 && sepArcsec > atEpochToleranceArc {
			t.Errorf("dt=0: %.4f\" from Horizons at the elements' own epoch, where two-body motion "+
				"is exact by construction — a frame or obliquity convention is wrong (#391)", sepArcsec)
		}

		if sepArcsec > toleranceArcsec {
			t.Errorf("dt=%+.0fd: angular separation %.3f\" exceeds %.1f\" tolerance", dtDays, sepArcsec, toleranceArcsec)
		}
	}

	// Nine outages is a comparison that did not happen, which is a skip and
	// not a pass.
	if compared == 0 {
		t.Skipf("Horizons did not answer for any of the %d epochs", outages)
	}

	t.Logf("max angular separation over +/-30 days: %.3f arcsec (%d of %d epochs compared)",
		maxSepArcsec, compared, compared+outages)
}
