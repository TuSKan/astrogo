//go:build network

package kepler_test

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/metrology"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

const horizonsHost = "ssd.jpl.nasa.gov:443"

// jovicentric fetches a satellite's position relative to Jupiter from
// Horizons, in kilometres, ICRF.
func jovicentric(t *testing.T, naif int, startTDB, stopTDB, step string) map[float64]vector.Vec3 {
	t.Helper()

	url := fmt.Sprintf("https://ssd.jpl.nasa.gov/api/horizons.api?format=text"+
		"&COMMAND='%d'&CENTER='@599'&MAKE_EPHEM='YES'&EPHEM_TYPE='VECTORS'"+
		"&START_TIME='%s'&STOP_TIME='%s'&STEP_SIZE='%s'&OUT_UNITS='KM-S'"+
		"&REF_PLANE='FRAME'&VEC_TABLE='2'&CSV_FORMAT='YES'&OBJ_DATA='NO'",
		naif, strings.ReplaceAll(startTDB, " ", "%20"),
		strings.ReplaceAll(stopTDB, " ", "%20"), step)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		testutil.SkipOnUpstreamFailure(t, err)
		t.Fatalf("Horizons: %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		testutil.SkipOnUpstreamFailure(t, err)
		t.Fatalf("read Horizons: %v", err)
	}

	out := make(map[float64]vector.Vec3)

	var inData bool

	sc := bufio.NewScanner(strings.NewReader(string(body)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())

		switch line {
		case "$$SOE":
			inData = true

			continue
		case "$$EOE":
			inData = false
		}

		if !inData {
			continue
		}

		cols := strings.Split(line, ",")
		if len(cols) < 5 {
			continue
		}

		jd, err := strconv.ParseFloat(strings.TrimSpace(cols[0]), 64)
		if err != nil {
			continue
		}

		var v [3]float64

		for i := range v {
			if v[i], err = strconv.ParseFloat(strings.TrimSpace(cols[2+i]), 64); err != nil {
				t.Fatalf("parse column %d: %v", 2+i, err)
			}
		}

		out[jd] = vector.V3(v[0], v[1], v[2])
	}

	return out
}

// TestGalileanAgainstHorizons measures what a Laplace-plane two-body orbit is
// actually worth against the real thing.
//
// # Why measure something known to be incomplete
//
// Because "incomplete" is not a number. The package doc says two-body motion
// diverges for these satellites under Jupiter's J₂ and their own resonance,
// and a reader deciding whether to use this needs to know whether that means
// kilometres or radii. It is measured here rather than described.
//
// The contract is deliberately loose and is not an accuracy claim: it is a
// bound that catches the frame being wrong, which is a different and much
// larger error than the perturbations. Reading these elements against the
// ecliptic misplaces Io by 16,500 km; the perturbation error is smaller, so
// a bound between the two separates a broken rotation from physics this
// package openly does not model.
func TestGalileanAgainstHorizons(t *testing.T) {
	ref := metrology.Reference{
		Kind:    metrology.KindHorizons,
		Name:    "JPL Horizons",
		Version: "VECTORS, jovicentric, ICRF",
		Source:  "https://ssd.jpl.nasa.gov/api/horizons.api",
		Dataset: "JUP365 against two-body propagation of JPL mean elements",
	}

	suite := metrology.NewSuite("ephemeris.kepler.galilean.laplace", ref,
		metrology.MustContract(10_000, "km",
			"not an accuracy claim, and deliberately sits between two known errors. Two-body "+
				"motion is wrong here by construction — Jupiter's J2 and the Laplace resonance "+
				"are unmodelled — and that drift measures 5,905 km at worst over the ten days "+
				"sampled. Reading the same elements against the ecliptic instead of the Laplace "+
				"plane displaces Io by 16,500 km. A bound above the first and below the second "+
				"fails when the frame is wrong and passes while the physics is merely absent, "+
				"which is the only distinction this suite can honestly make",
			"both figures measured: TestGalileanAgainstHorizons for the drift, "+
				"TestReadingLaplaceElementsAsEclipticIsBadlyWrong for the frame error"))

	if !testutil.Reachable(horizonsHost) {
		metrology.NotVerified(t, "JPL Horizons is unreachable", suite)
	}

	for i, s := range jovianMeanElements {
		el := jovianElements(t, i)

		want := jovicentric(t, 500+jovianNAIF[i], "2000-01-01 12:00 TDB", "2000-01-11 12:00", "1d")
		if len(want) == 0 {
			t.Logf("%s: Horizons returned no rows", s.name)

			continue
		}

		for jd, refPos := range want {
			pos, _, err := el.StateAt(time.FromJD(jd, time.TDB))
			if err != nil {
				t.Errorf("%s: StateAt: %v", s.name, err)

				continue
			}

			gotKM := pos.MulScalar(auMeters / 1e3)
			diff := gotKM.Sub(refPos).Norm()

			suite.Add(metrology.Sample{
				Error:   diff,
				Label:   s.name,
				Context: fmt.Sprintf("JD %.1f TDB, |r| = %.0f km", jd, refPos.Norm()),
			})
		}
	}

	if suite.Len() == 0 {
		metrology.NotVerified(t, "Horizons returned no comparable rows", suite)
	}

	suite.Report(t)
}

// jovianNAIF maps the table's rows to the satellite numbers Horizons uses:
// Io is 501, Europa 502, Ganymede 503, Callisto 504.
var jovianNAIF = []int{1, 2, 3, 4}
