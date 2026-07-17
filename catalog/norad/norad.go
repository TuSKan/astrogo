package norad

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	gotime "time"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

// ErrNoData is returned when no GP records exist for a catalog number.
var ErrNoData = errors.New("norad: no data for catalog number")

// GP represents a NORAD General Perturbations element set (OMM-compatible).
// Field names align with CCSDS 502.0-B-3 / Space Data Standards OMM schema.
type GP struct {
	ObjectName      string  `json:"OBJECT_NAME"`
	ObjectID        string  `json:"OBJECT_ID"`
	Epoch           string  `json:"EPOCH"`
	Classification  string  `json:"CLASSIFICATION_TYPE"`
	MeanAnomaly     float64 `json:"MEAN_ANOMALY"`
	Inclination     float64 `json:"INCLINATION"`
	RAOfAscNode     float64 `json:"RA_OF_ASC_NODE"`
	ArgOfPericenter float64 `json:"ARG_OF_PERICENTER"`
	Eccentricity    float64 `json:"ECCENTRICITY"`
	EphemerisType   int     `json:"EPHEMERIS_TYPE"`
	MeanMotion      float64 `json:"MEAN_MOTION"`
	NoradCatID      int     `json:"NORAD_CAT_ID"`
	ElementSetNo    int     `json:"ELEMENT_SET_NO"`
	RevAtEpoch      int     `json:"REV_AT_EPOCH"`
	BStar           float64 `json:"BSTAR"`
	MeanMotionDot   float64 `json:"MEAN_MOTION_DOT"`
	MeanMotionDDot  float64 `json:"MEAN_MOTION_DDOT"`
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
		epochYr = t.Year() % 100
		epochDay = t.DayOfYear()
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
	line1 += strconv.Itoa(checksumTLE(line1))

	// Line 2
	line2 = fmt.Sprintf("2 %05d %8.4f %8.4f %07d %8.4f %8.4f %11.8f%5d",
		catID, gp.Inclination, gp.RAOfAscNode,
		int(gp.Eccentricity*1e7), gp.ArgOfPericenter,
		gp.MeanAnomaly, gp.MeanMotion, gp.RevAtEpoch)
	line2 = padToLength(line2, 68)
	line2 += strconv.Itoa(checksumTLE(line2))

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

	var resultSb116 strings.Builder

	for i, part := range splitIntDes(id) {
		if i == 0 {
			// Take last 2 digits of year.
			if len(part) >= 4 {
				resultSb116.WriteString(part[2:])
			} else {
				resultSb116.WriteString(part)
			}
		} else {
			resultSb116.WriteString(part)
		}
	}

	result += resultSb116.String()

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
	// QueryCatNr queries by NORAD catalog number (1–9 digits).
	QueryCatNr QueryType = "CATNR"
	// QueryIntDes queries by international designator (yyyy-nnn).
	QueryIntDes QueryType = "INTDES"
	// QueryGroup queries by CelestTrak satellite group.
	QueryGroup QueryType = "GROUP"
	// QueryName queries by satellite name.
	QueryName QueryType = "NAME"
	// QuerySpecial queries special datasets (GPZ, DECAYING).
	QuerySpecial QueryType = "SPECIAL"
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
	client *remote.Client
	cache  resolve.Cache
}

// New returns a Provider configured with sensible defaults.
func New() *Provider {
	client, err := remote.NewClientFor(remote.CelesTrak)
	if err != nil {
		panic(err) // unregistered endpoint would be a programmer error
	}

	return &Provider{
		client: client,
		cache:  resolve.NewMapCache(),
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
	// No local timeout wrapper needed: NewClientFor(remote.CelesTrak) already
	// bounds the whole request at the endpoint's registered Timeout.
	gps, err := p.Fetch(context.Background(), QueryName, query)
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

	params := url.Values{}
	params.Set(string(query), value)
	params.Set("FORMAT", "JSON")

	var gps []GP
	if err := p.client.GetJSON(ctx, remote.CelesTrak, "", params, &gps); err != nil {
		return nil, fmt.Errorf("norad: fetch failed: %w", err)
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
	gps, err := p.Fetch(ctx, QueryCatNr, strconv.Itoa(catNr))
	if err != nil {
		return GP{}, err
	}

	if len(gps) == 0 {
		return GP{}, fmt.Errorf("%w: %d", ErrNoData, catNr)
	}

	return gps[0], nil
}

// gpToTarget converts a GP element set to a resolve.Target.
func gpToTarget(gp GP) resolve.Target {
	l1, l2 := gp.ToTLE()
	epoch, _ := gp.EpochTime()

	return resolve.Target{
		ID:          strconv.Itoa(gp.NoradCatID),
		Name:        gp.ObjectName,
		Designation: gp.ObjectID,
		Kind:        resolve.Kind("Satellite"),
		Catalog:     "norad",
		Epoch:       epoch,
		TLELine1:    l1,
		TLELine2:    l2,
	}
}
