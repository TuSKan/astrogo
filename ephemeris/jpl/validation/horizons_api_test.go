//go:build network

package jpl_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Static errors for the Horizons response parser. err113 wants these declared
// rather than built at the raise site, and the reason applies here: a caller
// deciding whether a validation failure is Horizons changing its output format
// or the network misbehaving has to be able to test for it, which errors.Is
// can only do against a value that exists.
var (
	errNoEphemerisData   = errors.New("horizons: no ephemeris data in the response")
	errNoVectorRows      = errors.New("horizons: no vector rows returned")
	errUnexpectedColumns = errors.New("horizons: unexpected column count")
	errColumnOutOfRange  = errors.New("horizons: column index out of range")
	errNoObserverRows    = errors.New("horizons: no observer rows returned")
)

// StateVector matches your desired JSON output
type StateVector struct {
	Body    string    `json:"body"`
	NaifID  int       `json:"naif_id"`
	Epoch   string    `json:"epoch"`
	ET      float64   `json:"et"`
	Pos     []float64 `json:"pos"`
	Vel     []float64 `json:"vel"`
	UnitPos string    `json:"unit_pos"`
	UnitVel string    `json:"unit_vel"`
}

// errHorizonsUnavailable marks a response that is not a Horizons API
// answer at all — ssd.jpl.nasa.gov intermittently serves its HTML error
// page under load. That is downtime, not wrong ephemeris data, so callers
// skip on it; a real Horizons answer with bad numbers still fails.
var errHorizonsUnavailable = errors.New("JPL Horizons served a non-API response")

// horizonsUnavailable reports whether body is JPL's web error page rather
// than the API's text output.
func horizonsUnavailable(body string) bool {
	head := strings.ToLower(strings.TrimSpace(body))

	return strings.HasPrefix(head, "<!doctype html") || strings.HasPrefix(head, "<html")
}

// fetchVector returns the geocentric state at one instant.
//
// # The scale of the returned ET, and the trap in it
//
// TIME_TYPE='UT' below changes the output column, not only the input: with it
// set, Horizons labels column 0 "JDUT — Julian Day Number, Universal Time"
// rather than the JDTDB it prints for a vector table by default. The
// conversion here treats that column as TDB, which is correct only because
// every caller suffixes startStr with an explicit scale — "2000-01-01 12:00
// TDB" — and the explicit suffix wins.
//
// Call it with a bare "2000-01-01 12:00" and the number that comes back is a
// UT Julian Date read as TDB: a 69-second error, silent, in a value that
// looks entirely reasonable. Verified live: with a suffixed start time this
// path agrees with DE440 to 5e-14 AU, and the corpus generator hit exactly
// that 69 seconds — about 1040 arcseconds of hour angle — when it used
// unsuffixed epochs and stored the column as TDB.
func fetchVector(naifID int, bodyName string, startStr, stopStr string) (*StateVector, error) {
	// 1. Define the base URL
	baseURL := "https://ssd.jpl.nasa.gov/api/horizons.api"

	// 2. Build the query parameters safely
	params := url.Values{}
	params.Add("format", "text")
	params.Add("COMMAND", fmt.Sprintf("'%d'", naifID))
	params.Add("CENTER", "'@399'")
	params.Add("MAKE_EPHEM", "'YES'")
	params.Add("EPHEM_TYPE", "'VECTORS'")
	params.Add("START_TIME", fmt.Sprintf("'%s'", startStr))
	params.Add("STOP_TIME", fmt.Sprintf("'%s'", stopStr))
	params.Add("STEP_SIZE", "'1d'")
	params.Add("OUT_UNITS", "'AU-D'")
	params.Add("REF_PLANE", "'FRAME'")
	params.Add("VEC_TABLE", "'2'")
	params.Add("CSV_FORMAT", "'YES'")
	params.Add("OBJ_DATA", "'NO'")
	params.Add("TIME_TYPE", "'UT'")

	// 3. Encode and fix spaces. (url.Values uses '+' for spaces, Horizons prefers '%20')
	encodedQuery := strings.ReplaceAll(params.Encode(), "+", "%20")
	reqURL := fmt.Sprintf("%s?%s", baseURL, encodedQuery)

	// 4. Execute the request
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
		return nil, fmt.Errorf("%w: %s", errNoEphemerisData, responseStr[:int(math.Min(float64(len(responseStr)), 500))])
	}

	csvBlock := responseStr[soeIdx+6 : eoeIdx]

	lines := strings.Split(strings.TrimSpace(csvBlock), "\n")
	if len(lines) == 0 {
		return nil, errNoVectorRows
	}

	cols := strings.Split(lines[0], ",")
	if len(cols) < 8 {
		return nil, errUnexpectedColumns
	}

	// Safely parse a specific index from the cols slice
	parseIdx := func(idx int) (float64, error) {
		if idx >= len(cols) {
			return 0, fmt.Errorf("%w: %d of %d", errColumnOutOfRange, idx, len(cols))
		}

		return strconv.ParseFloat(strings.TrimSpace(cols[idx]), 64)
	}

	// 1. Parse Time
	jdTDB, err := parseIdx(0)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JDTDB: %w", err)
	}

	etSeconds := (jdTDB - 2451545.0) * 86400.0

	// 2. Parse Positions (X, Y, Z)
	pos := make([]float64, 3)
	for i := range 3 {
		if pos[i], err = parseIdx(i + 2); err != nil {
			return nil, fmt.Errorf("failed to parse position axis %d: %w", i, err)
		}
	}

	// 3. Parse Velocities (VX, VY, VZ)
	vel := make([]float64, 3)
	for i := range 3 {
		if vel[i], err = parseIdx(i + 5); err != nil {
			return nil, fmt.Errorf("failed to parse velocity axis %d: %w", i, err)
		}
	}

	// 4. Build the final struct
	sv := &StateVector{
		Body:    bodyName,
		NaifID:  naifID,
		Epoch:   "2000-01-01T12:00:00Z",
		ET:      etSeconds,
		Pos:     pos,
		Vel:     vel,
		UnitPos: "AU",
		UnitVel: "AU/day",
	}

	return sv, nil
}

// ObserverPoint matches the quantities we pull from Horizons OBSERVER table
// fetchVectorSeries queries the Horizons API for geocentric state vectors
// across a time range, returning every row rather than only the first.
//
// The geocentric state does not depend on the observing site, so one series
// per body covers every site in a corpus at once. That is the difference
// between a few dozen requests to somebody else's server and a few hundred.
func fetchVectorSeries(naifID int, bodyName, startStr, stopStr, stepStr string) ([]StateVector, error) {
	baseURL := "https://ssd.jpl.nasa.gov/api/horizons.api"

	params := url.Values{}
	params.Add("format", "text")
	params.Add("COMMAND", fmt.Sprintf("'%d'", naifID))
	params.Add("CENTER", "'@399'")
	params.Add("MAKE_EPHEM", "'YES'")
	params.Add("EPHEM_TYPE", "'VECTORS'")
	params.Add("START_TIME", fmt.Sprintf("'%s'", startStr))
	params.Add("STOP_TIME", fmt.Sprintf("'%s'", stopStr))
	params.Add("STEP_SIZE", fmt.Sprintf("'%s'", stepStr))
	params.Add("OUT_UNITS", "'AU-D'")
	params.Add("REF_PLANE", "'FRAME'")
	params.Add("VEC_TABLE", "'2'")
	params.Add("CSV_FORMAT", "'YES'")
	params.Add("OBJ_DATA", "'NO'")
	params.Add("TIME_TYPE", "'UT'")

	encodedQuery := strings.ReplaceAll(params.Encode(), "+", "%20")

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		fmt.Sprintf("%s?%s", baseURL, encodedQuery), nil)
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

	lines := strings.Split(strings.TrimSpace(responseStr[soeIdx+6:eoeIdx]), "\n")
	out := make([]StateVector, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		cols := strings.Split(line, ",")
		if len(cols) < 8 {
			return nil, fmt.Errorf("%w: vector output had %d", errUnexpectedColumns, len(cols))
		}

		val := func(idx int) float64 {
			v, _ := strconv.ParseFloat(strings.TrimSpace(cols[idx]), 64)

			return v
		}

		sv := StateVector{
			Body:    bodyName,
			NaifID:  naifID,
			ET:      (val(0) - 2451545.0) * 86400.0,
			Pos:     []float64{val(2), val(3), val(4)},
			Vel:     []float64{val(5), val(6), val(7)},
			UnitPos: "AU",
			UnitVel: "AU/day",
		}

		out = append(out, sv)
	}

	if len(out) == 0 {
		return nil, errNoVectorRows
	}

	return out, nil
}

// ObserverPoint is one row of a Horizons OBSERVER table.
//
// Azimuth and Elevation are **airless** — unrefracted. The queries below send
// no APPARENT parameter, so Horizons applies its default of AIRLESS, and it
// says so in its own response header ("Atmos refraction: NO (AIRLESS)"),
// checked live for this exact query shape. The comparison tests accordingly
// build their coord.Context with atmosphere.RefractionNone.
//
// These two fields were previously documented as "Refracted", which is the
// opposite of what the service returns. Anyone reconciling that comment with
// the RefractionNone on the astrogo side would have concluded the comparison
// was mismatched by a refraction term of up to half a degree near the
// horizon, and gone looking for a bug that is not there.
type ObserverPoint struct {
	Body      string  `json:"body"`
	ET        float64 `json:"et"`
	AstroRA   float64 // Astrometric RA in degrees
	AstroDec  float64 // Astrometric Dec in degrees
	AppRA     float64 // Apparent RA in degrees
	AppDec    float64 // Apparent Dec in degrees
	Azimuth   float64 // Airless (unrefracted) azimuth in degrees
	Elevation float64 // Airless (unrefracted) elevation in degrees
	Range     float64 // Observer to Target Range in AU
}

// fetchObserverTable queries the Horizons API for a ground-based observer with
// QUANTITIES='1,2,4,20': astrometric RA/Dec, apparent RA/Dec, airless Az/El and
// observer range.
func fetchObserverTable(naifID int, bodyName string, lon, lat, height float64, startStr, stopStr string) (*ObserverPoint, error) {
	baseURL := "https://ssd.jpl.nasa.gov/api/horizons.api"

	params := url.Values{}
	params.Add("format", "text")
	params.Add("COMMAND", fmt.Sprintf("'%d'", naifID))
	params.Add("CENTER", "'coord@399'")                                          // Earth Observer
	params.Add("SITE_COORD", fmt.Sprintf("'%f,%f,%f'", lon, lat, height/1000.0)) // Longitude, Latitude, Elevation in km
	params.Add("MAKE_EPHEM", "'YES'")
	params.Add("EPHEM_TYPE", "'OBSERVER'")
	params.Add("START_TIME", fmt.Sprintf("'%s'", startStr))
	params.Add("STOP_TIME", fmt.Sprintf("'%s'", stopStr))
	params.Add("STEP_SIZE", "'1m'") // 1 minute step size

	// Important quantities: 1 (Astrometric RA/Dec), 2 (Apparent RA/Dec), 4 (Azi/Elev), 20 (Observer Range)
	params.Add("QUANTITIES", "'1,2,4,20'")
	params.Add("CAL_FORMAT", "'JD'")
	params.Add("CSV_FORMAT", "'YES'")
	params.Add("OBJ_DATA", "'NO'")

	encodedQuery := strings.ReplaceAll(params.Encode(), "+", "%20")
	reqURL := fmt.Sprintf("%s?%s", baseURL, encodedQuery)

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

	csvBlock := responseStr[soeIdx+6 : eoeIdx]

	lines := strings.Split(strings.TrimSpace(csvBlock), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return nil, errNoObserverRows
	}

	cols := strings.Split(lines[0], ",")
	// JD, SolarPresence, LunarPresence, Astrometric RA, Dec, Apparent RA, Dec, Azimuth, Elevation
	// Horizons usually renders: JDTDB, Calendar, ... flags
	// For standard CSV output without datetime string:
	// JD, AstRA, AstDec, AppRA, AppDec, Az, El

	// Because of flags, sometimes it's larger. Let's just parse backwards or specifically:
	// Format is typically: JD, target_presence_flags, RA(ICRF), DEC(ICRF), RA(a-app), DEC(a-app), Azi(a-app), Elev(a-app)

	if len(cols) < 8 {
		return nil, fmt.Errorf("%w: observer output had %d", errUnexpectedColumns, len(cols))
	}

	// Helper to extract cleanly parsed floats from the end of the array
	cLen := len(cols)
	parseFloat := func(idx int) float64 {
		v, _ := strconv.ParseFloat(strings.TrimSpace(cols[idx]), 64)
		return v
	}

	parseAngleStr := func(idx int, isRA bool) float64 {
		s := strings.TrimSpace(cols[idx])

		parts := strings.Fields(s)
		if len(parts) != 3 {
			return 0
		}

		d, _ := strconv.ParseFloat(parts[0], 64)
		m, _ := strconv.ParseFloat(parts[1], 64)
		sec, _ := strconv.ParseFloat(parts[2], 64)

		sign := 1.0
		if strings.HasPrefix(s, "-") {
			sign = -1.0
			d = math.Abs(d)
		}

		val := d + m/60.0 + sec/3600.0
		if isRA {
			val *= 15.0 // Convert hours to degrees
		}

		return sign * val
	}

	target := &ObserverPoint{
		Body:      bodyName,
		AstroRA:   parseAngleStr(cLen-9, true),
		AstroDec:  parseAngleStr(cLen-8, false),
		AppRA:     parseAngleStr(cLen-7, true),
		AppDec:    parseAngleStr(cLen-6, false),
		Azimuth:   parseFloat(cLen - 5),
		Elevation: parseFloat(cLen - 4),
		Range:     parseFloat(cLen - 3), // Quantity 20 adds Range and Range Rate, trailing comma creates empty elem
	}

	return target, nil
}

// parseObserverRow decodes one CSV row of a Horizons OBSERVER-table
// response into an ObserverPoint. The column layout is read from the END
// of the row (cLen-9, cLen-8, ...) rather than fixed positions, because
// the leading columns vary in count depending on which optional
// solar/lunar-presence flag columns Horizons includes — see
// fetchObserverTable's own comment for the confirmed real layout. Shared
// by fetchObserverTable (single-row) and fetchObserverSeries (every row).
func parseObserverRow(cols []string, bodyName string) ObserverPoint {
	cLen := len(cols)

	parseFloat := func(idx int) float64 {
		v, _ := strconv.ParseFloat(strings.TrimSpace(cols[idx]), 64)
		return v
	}

	parseAngleStr := func(idx int, isRA bool) float64 {
		s := strings.TrimSpace(cols[idx])

		parts := strings.Fields(s)
		if len(parts) != 3 {
			return 0
		}

		d, _ := strconv.ParseFloat(parts[0], 64)
		m, _ := strconv.ParseFloat(parts[1], 64)
		sec, _ := strconv.ParseFloat(parts[2], 64)

		sign := 1.0
		if strings.HasPrefix(s, "-") {
			sign = -1.0
			d = math.Abs(d)
		}

		val := d + m/60.0 + sec/3600.0
		if isRA {
			val *= 15.0
		}

		return sign * val
	}

	// CAL_FORMAT='JD' (set by both callers) guarantees cols[0] is the bare
	// Julian Date, regardless of how many presence-flag columns follow it —
	// same JD-to-ET conversion fetchVector uses.
	etSeconds := (parseFloat(0) - 2451545.0) * 86400.0

	return ObserverPoint{
		Body:      bodyName,
		ET:        etSeconds,
		AstroRA:   parseAngleStr(cLen-9, true),
		AstroDec:  parseAngleStr(cLen-8, false),
		AppRA:     parseAngleStr(cLen-7, true),
		AppDec:    parseAngleStr(cLen-6, false),
		Azimuth:   parseFloat(cLen - 5),
		Elevation: parseFloat(cLen - 4),
		Range:     parseFloat(cLen - 3),
	}
}

// fetchObserverSeries queries the Horizons API for a ground-based observer
// across a time range (START_TIME/STOP_TIME/STEP_SIZE), returning every row
// of the OBSERVER table — unlike fetchObserverTable, which only parses
// lines[0]. This lets the precision-floor matrix cover many epochs per
// (body, site) pair in a single request instead of one request per epoch.
func fetchObserverSeries(naifID int, bodyName string, lon, lat, height float64, startStr, stopStr, stepStr string) ([]ObserverPoint, error) {
	baseURL := "https://ssd.jpl.nasa.gov/api/horizons.api"

	params := url.Values{}
	params.Add("format", "text")
	params.Add("COMMAND", fmt.Sprintf("'%d'", naifID))
	params.Add("CENTER", "'coord@399'") // Earth Observer
	params.Add("SITE_COORD", fmt.Sprintf("'%f,%f,%f'", lon, lat, height/1000.0))
	params.Add("MAKE_EPHEM", "'YES'")
	params.Add("EPHEM_TYPE", "'OBSERVER'")
	params.Add("START_TIME", fmt.Sprintf("'%s'", startStr))
	params.Add("STOP_TIME", fmt.Sprintf("'%s'", stopStr))
	params.Add("STEP_SIZE", fmt.Sprintf("'%s'", stepStr))
	params.Add("QUANTITIES", "'1,2,4,20'")
	params.Add("CAL_FORMAT", "'JD'")
	params.Add("CSV_FORMAT", "'YES'")
	params.Add("OBJ_DATA", "'NO'")

	encodedQuery := strings.ReplaceAll(params.Encode(), "+", "%20")
	reqURL := fmt.Sprintf("%s?%s", baseURL, encodedQuery)

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

	soeIdx := strings.Index(responseStr, "$$SOE")

	eoeIdx := strings.Index(responseStr, "$$EOE")
	if soeIdx == -1 || eoeIdx == -1 {
		return nil, errNoEphemerisData
	}

	csvBlock := responseStr[soeIdx+6 : eoeIdx]
	lines := strings.Split(strings.TrimSpace(csvBlock), "\n")

	points := make([]ObserverPoint, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		cols := strings.Split(line, ",")
		if len(cols) < 8 {
			return nil, fmt.Errorf("%w: observer output had %d", errUnexpectedColumns, len(cols))
		}

		points = append(points, parseObserverRow(cols, bodyName))
	}

	if len(points) == 0 {
		return nil, errNoObserverRows
	}

	return points, nil
}
