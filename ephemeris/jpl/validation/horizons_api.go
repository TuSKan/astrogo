package jpl_test

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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

func fetchVector(naifID int, bodyName string) (*StateVector, error) {
	// 1. Define the base URL
	baseURL := "https://ssd.jpl.nasa.gov/api/horizons.api"

	// 2. Build the query parameters safely
	params := url.Values{}
	params.Add("format", "text")
	params.Add("COMMAND", fmt.Sprintf("'%d'", naifID))
	params.Add("CENTER", "'@399'")
	params.Add("MAKE_EPHEM", "'YES'")
	params.Add("EPHEM_TYPE", "'VECTORS'")
	params.Add("START_TIME", "'2000-01-01 12:00 TDB'")
	params.Add("STOP_TIME", "'2000-01-01 12:01'")
	params.Add("STEP_SIZE", "'1d'")
	params.Add("OUT_UNITS", "'AU-D'")
	params.Add("REF_PLANE", "'FRAME'")
	params.Add("VEC_TABLE", "'2'")
	params.Add("CSV_FORMAT", "'YES'")
	params.Add("OBJ_DATA", "'NO'")

	// 3. Encode and fix spaces. (url.Values uses '+' for spaces, Horizons prefers '%20')
	encodedQuery := strings.ReplaceAll(params.Encode(), "+", "%20")
	reqURL := fmt.Sprintf("%s?%s", baseURL, encodedQuery)

	// 4. Execute the request
	resp, err := http.Get(reqURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	responseStr := string(bodyBytes)

	soeIdx := strings.Index(responseStr, "$$SOE")
	eoeIdx := strings.Index(responseStr, "$$EOE")
	if soeIdx == -1 || eoeIdx == -1 {
		return nil, fmt.Errorf("ephemeris data not found in response")
	}

	csvBlock := responseStr[soeIdx+6 : eoeIdx]
	lines := strings.Split(strings.TrimSpace(csvBlock), "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("no vector lines found")
	}

	cols := strings.Split(lines[0], ",")
	if len(cols) < 8 {
		return nil, fmt.Errorf("unexpected column count")
	}

	// Safely parse a specific index from the cols slice
	parseIdx := func(idx int) (float64, error) {
		if idx >= len(cols) {
			return 0, fmt.Errorf("index %d out of bounds for cols length %d", idx, len(cols))
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
	for i := 0; i < 3; i++ {
		if pos[i], err = parseIdx(i + 2); err != nil {
			return nil, fmt.Errorf("failed to parse position axis %d: %w", i, err)
		}
	}

	// 3. Parse Velocities (VX, VY, VZ)
	vel := make([]float64, 3)
	for i := 0; i < 3; i++ {
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
type ObserverPoint struct {
	Body      string  `json:"body"`
	ET        float64 `json:"et"`
	AstroRA   float64 // Astrometric RA in degrees
	AstroDec  float64 // Astrometric Dec in degrees
	AppRA     float64 // Apparent RA in degrees
	AppDec    float64 // Apparent Dec in degrees
	Azimuth   float64 // Refracted Azimuth in degrees
	Elevation float64 // Refracted Elevation in degrees
}

// fetchObserverTable queries the Horizons API for a ground-based observer
// QUANTITIES='1,2,4' corresponding to Astrometric RA/Dec, Apparent RA/Dec, and Az/El.
func fetchObserverTable(naifID int, bodyName string, lon, lat, height float64, startStr, stopStr string) (*ObserverPoint, error) {
	baseURL := "https://ssd.jpl.nasa.gov/api/horizons.api"

	params := url.Values{}
	params.Add("format", "text")
	params.Add("COMMAND", fmt.Sprintf("'%d'", naifID))
	params.Add("CENTER", "'coord@399'") // Earth Observer
	params.Add("SITE_COORD", fmt.Sprintf("'%f,%f,%f'", lon, lat, height/1000.0)) // Longitude, Latitude, Elevation in km
	params.Add("MAKE_EPHEM", "'YES'")
	params.Add("EPHEM_TYPE", "'OBSERVER'")
	params.Add("START_TIME", fmt.Sprintf("'%s'", startStr))
	params.Add("STOP_TIME", fmt.Sprintf("'%s'", stopStr)) 
    params.Add("STEP_SIZE", "'1m'") // 1 minute step size

	// Important quantities: 1 (Astrometric RA/Dec), 2 (Apparent RA/Dec), 4 (Azi/Elev)
	params.Add("QUANTITIES", "'1,2,4'")
	params.Add("CAL_FORMAT", "'JD'")
	params.Add("CSV_FORMAT", "'YES'")
	params.Add("OBJ_DATA", "'NO'")

	encodedQuery := strings.ReplaceAll(params.Encode(), "+", "%20")
	reqURL := fmt.Sprintf("%s?%s", baseURL, encodedQuery)

	resp, err := http.Get(reqURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	responseStr := string(bodyBytes)

	soeIdx := strings.Index(responseStr, "$$SOE")
	eoeIdx := strings.Index(responseStr, "$$EOE")
	if soeIdx == -1 || eoeIdx == -1 {
		return nil, fmt.Errorf("ephemeris data not found in response")
	}

	csvBlock := responseStr[soeIdx+6 : eoeIdx]
	lines := strings.Split(strings.TrimSpace(csvBlock), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return nil, fmt.Errorf("no observer lines found")
	}

	cols := strings.Split(lines[0], ",")
	// JD, SolarPresence, LunarPresence, Astrometric RA, Dec, Apparent RA, Dec, Azimuth, Elevation
	// Horizons usually renders: JDTDB, Calendar, ... flags
	// For standard CSV output without datetime string:
	// JD, AstRA, AstDec, AppRA, AppDec, Az, El
	
	// Because of flags, sometimes it's larger. Let's just parse backwards or specifically:
	// Format is typically: JD, target_presence_flags, RA(ICRF), DEC(ICRF), RA(a-app), DEC(a-app), Azi(a-app), Elev(a-app)
	
	if len(cols) < 8 {
		return nil, fmt.Errorf("unexpected column count in observer output: %d", len(cols))
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
		AstroRA:   parseAngleStr(cLen - 7, true),
		AstroDec:  parseAngleStr(cLen - 6, false),
		AppRA:     parseAngleStr(cLen - 5, true),
		AppDec:    parseAngleStr(cLen - 4, false),
		Azimuth:   parseFloat(cLen - 3),
		Elevation: parseFloat(cLen - 2),
	}

	return target, nil
}
