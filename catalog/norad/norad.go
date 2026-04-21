package norad

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	gotime "time"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/time"
)

// CelestTrak GP API base URL.
var gpAPIBase = "https://celestrak.org/NORAD/elements/gp.php"

// GP represents a NORAD General Perturbations element set (OMM-compatible).
// Field names align with CCSDS 502.0-B-3 / Space Data Standards OMM schema.
type GP struct {
	ObjectName      string  `json:"OBJECT_NAME"`       // Satellite name
	ObjectID        string  `json:"OBJECT_ID"`         // International designator (yyyy-nnnp)
	Epoch           string  `json:"EPOCH"`             // Element set epoch (ISO 8601)
	MeanMotion      float64 `json:"MEAN_MOTION"`       // Revolutions per day
	Eccentricity    float64 `json:"ECCENTRICITY"`      // Dimensionless
	Inclination     float64 `json:"INCLINATION"`       // Degrees
	RAOfAscNode     float64 `json:"RA_OF_ASC_NODE"`    // Degrees
	ArgOfPericenter float64 `json:"ARG_OF_PERICENTER"` // Degrees
	MeanAnomaly     float64 `json:"MEAN_ANOMALY"`      // Degrees
	EphemerisType   int     `json:"EPHEMERIS_TYPE"`
	Classification  string  `json:"CLASSIFICATION_TYPE"` // U/C/S
	NoradCatID      int     `json:"NORAD_CAT_ID"`        // Up to 9 digits
	ElementSetNo    int     `json:"ELEMENT_SET_NO"`
	RevAtEpoch      int     `json:"REV_AT_EPOCH"`
	BStar           float64 `json:"BSTAR"`            // Drag coefficient (1/earth radii)
	MeanMotionDot   float64 `json:"MEAN_MOTION_DOT"`  // First derivative (rev/day²)
	MeanMotionDDot  float64 `json:"MEAN_MOTION_DDOT"` // Second derivative (rev/day³)
}

// EpochTime parses the GP epoch string into an astrogo Time (UTC).
func (gp GP) EpochTime() (time.Time, error) {
	t, err := gotime.Parse("2006-01-02T15:04:05.999999", gp.Epoch)
	if err != nil {
		// Try without fractional seconds.
		t, err = gotime.Parse("2006-01-02T15:04:05", gp.Epoch)
		if err != nil {
			return time.Time{}, fmt.Errorf("norad: cannot parse epoch %q: %w", gp.Epoch, err)
		}
	}
	return time.Date(t.Year(), t.Month(), t.Day(),
		t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.LocationUTC), nil
}

// ToTLE generates a TLE line pair from the GP data.
// This is useful for passing to SGP4 propagators that expect TLE format.
// The output strictly follows the fixed-column format of Spacetrack Report #3.
func (gp GP) ToTLE() (line1, line2 string) {
	// Compute epoch in TLE format: 2-digit year + day-of-year with fraction.
	epochYr := 0
	epochDay := 0.0
	if t, err := gp.EpochTime(); err == nil {
		jd1, jd2 := t.JDParts()
		year := t.Year()
		epochYr = year % 100
		jan1 := time.Date(year, 1, 1, 0, 0, 0, 0, time.LocationUTC)
		jdJan1_1, jdJan1_2 := jan1.JDParts()
		epochDay = (jd1 - jdJan1_1) + (jd2 - jdJan1_2) + 1.0
	}

	class := gp.Classification
	if class == "" {
		class = "U"
	}

	catID := min(gp.NoradCatID, 99999)

	// Convert international designator from yyyy-nnnp to TLE format (yynnnp, 8 chars).
	intdes := formatIntDes(gp.ObjectID)

	// Format ndot: implied decimal, right-justified in 10 chars.
	// Example: 0.00010082 → " .00010082"
	ndotStr := formatNdot(gp.MeanMotionDot)

	// Format nddot and bstar in TLE exponential notation.
	nddotStr := formatTLEExp(gp.MeanMotionDDot)
	bstarStr := formatTLEExp(gp.BStar)

	// Line 1: columns are fixed-width, total 69 chars including checksum.
	//   1 NNNNNC NNNNNAAA NNNNN.NNNNNNNN +.NNNNNNNN +NNNNN-N +NNNNN-N N NNNNN
	line1 = fmt.Sprintf("1 %05d%s %-8s %02d%012.8f %s %s %s %d %4d",
		catID, class, intdes, epochYr, epochDay,
		ndotStr, nddotStr, bstarStr,
		gp.EphemerisType, gp.ElementSetNo)
	line1 = padToLength(line1, 68)
	line1 += fmt.Sprintf("%d", checksumTLE(line1))

	// Line 2
	line2 = fmt.Sprintf("2 %05d %8.4f %8.4f %07d %8.4f %8.4f %11.8f%5d",
		catID, gp.Inclination, gp.RAOfAscNode,
		int(gp.Eccentricity*1e7), gp.ArgOfPericenter,
		gp.MeanAnomaly, gp.MeanMotion, gp.RevAtEpoch)
	line2 = padToLength(line2, 68)
	line2 += fmt.Sprintf("%d", checksumTLE(line2))

	return line1, line2
}

// formatIntDes converts an international designator from OMM format (yyyy-nnnp)
// to TLE format (yynnnp, padded to 8 chars).
func formatIntDes(id string) string {
	if len(id) < 4 {
		return fmt.Sprintf("%-8s", id)
	}
	// Remove hyphen: "1998-067A" → "98067A"
	result := ""
	for i, part := range splitIntDes(id) {
		if i == 0 {
			// Take last 2 digits of year.
			if len(part) >= 4 {
				result += part[2:]
			} else {
				result += part
			}
		} else {
			result += part
		}
	}
	return fmt.Sprintf("%-8s", result)
}

// splitIntDes splits "1998-067A" into ["1998", "067A"].
func splitIntDes(id string) []string {
	for i, c := range id {
		if c == '-' {
			return []string{id[:i], id[i+1:]}
		}
	}
	return []string{id}
}

// formatNdot formats the mean motion first derivative for TLE Line 1.
// Uses implied decimal format: 0.00010082 → " .00010082"
func formatNdot(val float64) string {
	if val >= 0 {
		return fmt.Sprintf(" .%08d", int(val*1e8+0.5))
	}
	return fmt.Sprintf("-.%08d", int(-val*1e8+0.5))
}

// formatTLEExp formats a float in TLE exponential notation.
// Examples:
//
//	0         → " 00000-0"
//	0.00019194 → " 19194-3"
//	-0.00019194 → "-19194-3"
func formatTLEExp(val float64) string {
	if val == 0 {
		return " 00000-0"
	}
	sign := " "
	if val < 0 {
		sign = "-"
		val = -val
	}

	exp := 0
	for val >= 1.0 {
		val /= 10.0
		exp++
	}
	for val < 0.1 && val > 0 {
		val *= 10.0
		exp--
	}

	mantissa := int(val*100000 + 0.5)
	return fmt.Sprintf("%s%05d%+d", sign, mantissa, exp)
}

// padToLength pads a string with spaces to the target length.
func padToLength(s string, length int) string {
	for len(s) < length {
		s += " "
	}
	if len(s) > length {
		s = s[:length]
	}
	return s
}

// checksumTLE computes the TLE modulo-10 checksum.
func checksumTLE(line string) int {
	sum := 0
	for _, c := range line {
		if c >= '0' && c <= '9' {
			sum += int(c - '0')
		} else if c == '-' {
			sum++
		}
	}
	return sum % 10
}

// QueryType identifies the CelestTrak GP query parameter.
type QueryType string

const (
	QueryCatNr   QueryType = "CATNR"   // Catalog Number (1–9 digits)
	QueryIntDes  QueryType = "INTDES"  // International Designator (yyyy-nnn)
	QueryGroup   QueryType = "GROUP"   // CelestTrak satellite group
	QueryName    QueryType = "NAME"    // Satellite name search
	QuerySpecial QueryType = "SPECIAL" // Special datasets (GPZ, DECAYING)
)

// Well-known CelestTrak GROUP values.
const (
	GroupStations  = "STATIONS" // Space stations
	GroupStarlink  = "STARLINK" // SpaceX Starlink
	GroupActive    = "ACTIVE"   // All active satellites
	GroupWeather   = "WEATHER"  // Weather satellites
	GroupGPSOps    = "GPS-OPS"  // GPS operational
	GroupGalileOps = "GALILEO"  // Galileo navigation
	GroupAmateur   = "AMATEUR"  // Amateur radio
	GroupVisible   = "VISUAL"   // Brightest / visually interesting
	GroupAnalyst   = "ANALYST"  // Analyst objects (incl. 6-digit catalog numbers)
)

// Provider implements resolve.Provider for NORAD satellite catalog lookups.
type Provider struct {
	client *resolve.Client
	cache  resolve.Cache
}

// New returns a Provider configured with sensible defaults.
func New() *Provider {
	return &Provider{
		client: resolve.NewClient(),
		cache:  resolve.NewArrowCache(),
	}
}

// Name returns the provider identifier.
func (p *Provider) Name() string { return "norad" }

// Capabilities returns the provider's supported capabilities.
func (p *Provider) Capabilities() []resolve.Capability {
	return []resolve.Capability{resolve.CapObjectResolution}
}

// Resolve attempts to find a single satellite by name or catalog number.
func (p *Provider) Resolve(query string) (resolve.Target, bool) {
	targets := p.Search(query)
	if len(targets) > 0 {
		return targets[0], true
	}
	return resolve.Target{}, false
}

// Search returns satellites matching the query string.
func (p *Provider) Search(query string) []resolve.Target {
	ctx, cancel := context.WithTimeout(context.Background(), 30*gotime.Second)
	defer cancel()

	gps, err := p.Fetch(ctx, QueryName, query)
	if err != nil || len(gps) == 0 {
		return nil
	}

	targets := make([]resolve.Target, 0, len(gps))
	for _, gp := range gps {
		targets = append(targets, gpToTarget(gp))
	}
	return targets
}

// Fetch queries the CelestTrak GP API and returns parsed element sets.
// It uses the JSON format for compact, natively-typed responses.
func (p *Provider) Fetch(ctx context.Context, query QueryType, value string) ([]GP, error) {
	cacheKey := fmt.Sprintf("norad:%s:%s", query, value)

	// NOTE: cache stores resolve.Target values, not GP structs.
	// GP data is always fetched fresh from the API.

	api, err := url.Parse(gpAPIBase)
	if err != nil {
		return nil, fmt.Errorf("norad: %w", err)
	}
	params := api.Query()
	params.Set(string(query), value)
	params.Set("FORMAT", "JSON")
	api.RawQuery = params.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, api.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("norad: %w", err)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("norad: fetch failed: %w", err)
	}
	defer resp.Body.Close()

	var gps []GP
	if err := json.NewDecoder(resp.Body).Decode(&gps); err != nil {
		return nil, fmt.Errorf("norad: json decode: %w", err)
	}

	// Cache as targets for the resolver layer.
	targets := make([]resolve.Target, len(gps))
	for i, gp := range gps {
		targets[i] = gpToTarget(gp)
	}
	_ = p.cache.Set(cacheKey, targets)

	return gps, nil
}

// FetchByID fetches GP data for a single NORAD catalog number.
func (p *Provider) FetchByID(ctx context.Context, catNr int) (GP, error) {
	gps, err := p.Fetch(ctx, QueryCatNr, fmt.Sprintf("%d", catNr))
	if err != nil {
		return GP{}, err
	}
	if len(gps) == 0 {
		return GP{}, fmt.Errorf("norad: no data for catalog number %d", catNr)
	}
	return gps[0], nil
}

// gpToTarget converts a GP element set to a resolve.Target.
func gpToTarget(gp GP) resolve.Target {
	l1, l2 := gp.ToTLE()
	epoch, _ := gp.EpochTime()
	return resolve.Target{
		ID:          fmt.Sprintf("%d", gp.NoradCatID),
		Name:        gp.ObjectName,
		Designation: gp.ObjectID,
		Kind:        resolve.Kind("Satellite"),
		Catalog:     "norad",
		Epoch:       epoch,
		TLELine1:    l1,
		TLELine2:    l2,
	}
}
