package norad

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

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
	t, err := time.Parse("2006-01-02T15:04:05.999999", gp.Epoch)
	if err != nil {
		// Try without fractional seconds.
		t, err = time.Parse("2006-01-02T15:04:05", gp.Epoch)
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
	line2 = fmt.Sprintf("2 %05d %8.4f %8.4f %s %8.4f %8.4f %11.8f%5d",
		catID, gp.Inclination, gp.RAOfAscNode,
		eccentricityField(gp.Eccentricity), gp.ArgOfPericenter,
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

// The TLE's numeric fields are written from the decimal digits a GP value was
// published in, not by scaling the float.
//
// CelesTrak publishes each element set both as GP JSON and as a TLE, and over
// 11,357 satellites the TLE's digits are the JSON's: eccentricity truncated to
// seven, B* and the second derivative rounded half up to a five-digit
// mantissa. Scaling the float first loses that. 0.0005799 times 1e7 is
// 5798.999999..., which truncated to 0005798 in 72 of them, and a B* whose
// sixth digit was exactly 5 rounded whichever way its binary value happened to
// fall, in 610 (#609). strconv's shortest representation of a float is the
// decimal it was parsed from, so the digits are taken from that.

// eccentricityField writes e as the seven digits after its decimal point,
// truncated, as the published field is: 0.00014417 is "0001441". An
// eccentricity outside [0, 1) is no orbit a TLE can carry, and is written as it
// always was, so that the validator rejects the line rather than this
// inventing a value for it.
func eccentricityField(e float64) string {
	if !(e >= 0 && e < 1) {
		return fmt.Sprintf("%07d", int(e*1e7))
	}

	_, frac, _ := strings.Cut(strconv.FormatFloat(e, 'f', -1, 64), ".")

	return (frac + "0000000")[:7]
}

// formatTLEExp formats a float in TLE exponential notation: a sign, a
// five-digit mantissa with an implied leading decimal point, and a signed
// one-digit exponent, rounded half up from the value's decimal digits.
// Examples:
//
//	0           → " 00000+0"
//	0.00019194  → " 19194-3"
//	-0.00019194 → "-19194-3"
//	3.10545e-7  → " 31055-6"
//
// Zero is written "+0", as CelesTrak's published TLEs write it.
func formatTLEExp(val float64) string {
	if val == 0 {
		return " 00000+0"
	}

	sign := " "
	if val < 0 {
		sign = "-"
		val = -val
	}

	// "7.72485e-06": digits "772485", and the value is 0.772485 x 10^-5.
	mant, expText, _ := strings.Cut(strconv.FormatFloat(val, 'e', -1, 64), "e")
	digits := strings.Replace(mant, ".", "", 1) + "000000"

	exp, _ := strconv.Atoi(expText)
	exp++

	mantissa, _ := strconv.Atoi(digits[:5])
	if digits[5] >= '5' {
		mantissa++
	}

	if mantissa == 100000 {
		mantissa = 10000
		exp++
	}

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
	client := remote.Default()

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

// Resolve finds the satellite a name or catalog number refers to.
//
// # Why this is not Search's first result
//
// It was, and CelesTrak's NAME query is a substring match. Asking for "ISS"
// returns eighteen objects — UME (ISS), UME-2 (ISS-B), SWISSCUBE, AISSAT 1
// and the five real station modules among them — with the Japanese
// Ionosphere Sounding Satellite first and ISS (ZARYA) third. Taking the first
// row meant tracking the wrong satellite, silently, for the single most
// common query this provider will ever receive.
//
// A catalog number now goes to CelesTrak's CATNR parameter, which is exact:
// "25544" used to find nothing at all, because the number was being sent as
// a name.
//
// A name is ranked rather than taken in arrival order — see [rankByName].
func (p *Provider) Resolve(ctx context.Context, query string) (resolve.Target, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return resolve.Target{}, fmt.Errorf("%w: empty query", resolve.ErrNotFound)
	}

	// A bare catalog number is an exact identifier; nothing to rank.
	if isCatalogNumber(q) {
		gps, err := p.Fetch(ctx, QueryCatNr, q)
		if err != nil {
			// Previously `err != nil || len(gps) == 0` collapsed both arms
			// into "false", so a CelesTrak outage and an unknown catalog
			// number were the same answer.
			return resolve.Target{}, err
		}

		if len(gps) == 0 {
			return resolve.Target{}, fmt.Errorf("%w: catalog number %q", resolve.ErrNotFound, q)
		}

		return gpToTarget(gps[0]), nil
	}

	targets, err := p.Search(ctx, q)
	if err != nil {
		return resolve.Target{}, err
	}

	if len(targets) == 0 {
		return resolve.Target{}, fmt.Errorf("%w: %q in CelesTrak", resolve.ErrNotFound, q)
	}

	return targets[0], nil
}

// isCatalogNumber reports whether the query is a bare NORAD catalog number.
// CelesTrak documents CATNR as 1-9 digits.
func isCatalogNumber(q string) bool {
	if len(q) == 0 || len(q) > 9 {
		return false
	}

	for _, r := range q {
		if r < '0' || r > '9' {
			return false
		}
	}

	return true
}

// rankByName orders a substring match so the satellite actually asked for
// comes first.
//
// CelesTrak matches anywhere in the name, so a short query pulls in
// unrelated craft: "ISS" reaches SWISSCUBE and AISSAT 1 through the middle of
// a word. The ordering is, in turn:
//
//   - an exact name match, which settles it outright;
//   - a name whose first word is the query — "ISS (ZARYA)" for "ISS" —
//     because a designation in parentheses is a component of the thing named
//     before it;
//   - a name beginning with the query;
//   - everything else, which is a mid-word match and almost never meant.
//
// Within a tier, the lower catalog number wins. For the station that is
// ISS (ZARYA), 25544 — the first module launched and the one every ephemeris
// means by "the ISS".
func rankByName(query string, targets []resolve.Target) {
	tier := func(name string) int {
		upper := strings.ToUpper(strings.TrimSpace(name))
		q := strings.ToUpper(strings.TrimSpace(query))

		switch {
		case upper == q:
			return 0
		case strings.HasPrefix(upper, q+" ("):
			return 1
		case strings.HasPrefix(upper, q):
			return 2
		default:
			return 3
		}
	}

	sort.SliceStable(targets, func(i, j int) bool {
		ti, tj := tier(targets[i].Name), tier(targets[j].Name)
		if ti != tj {
			return ti < tj
		}

		ni, erri := strconv.Atoi(targets[i].ID)
		nj, errj := strconv.Atoi(targets[j].ID)

		if erri == nil && errj == nil {
			return ni < nj
		}

		return targets[i].Name < targets[j].Name
	})
}

// Search returns satellites matching the query string.
func (p *Provider) Search(ctx context.Context, query string) ([]resolve.Target, error) {
	// No local timeout wrapper needed: NewClientFor(remote.CelesTrak) already
	// bounds the whole request at the endpoint's registered Timeout.
	gps, err := p.Fetch(ctx, QueryName, query)
	if err != nil {
		return nil, err
	}

	if len(gps) == 0 {
		return nil, nil
	}

	targets := make([]resolve.Target, 0, len(gps))
	for _, gp := range gps {
		targets = append(targets, gpToTarget(gp))
	}

	rankByName(query, targets)

	return targets, nil
}

// Fetch queries the CelestTrak GP API and returns parsed element sets.
// It uses the JSON format for compact, natively-typed responses. A query
// CelesTrak has no data for returns no element sets and a nil error.
func (p *Provider) Fetch(ctx context.Context, query QueryType, value string) ([]GP, error) {
	cacheKey := fmt.Sprintf("norad:%s:%s", query, value)

	// NOTE: cache stores resolve.Target values, not GP structs.
	// GP data is always fetched fresh from the API.

	params := url.Values{}
	params.Set(string(query), value)
	params.Set("FORMAT", "JSON")

	var gps []GP
	if err := p.client.GetJSON(ctx, remote.CelesTrak, "", params, &gps); err != nil {
		if isNoGPData(err) {
			return nil, nil
		}

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

// isNoGPData reports whether err is CelesTrak's answer to a query nothing
// matches: HTTP 404 with the plain-text body "No GP data found", checked
// live for an unknown NAME and an unknown CATNR. It is an empty result, not
// a failure; any other 404 still is one.
func isNoGPData(err error) bool {
	httpErr, ok := errors.AsType[*remote.HTTPError](err)

	return ok && httpErr.StatusCode == http.StatusNotFound && strings.TrimSpace(httpErr.Body) == "No GP data found"
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
		Kind:        resolve.KindSatellite,
		Catalog:     "norad",
		Epoch:       epoch,
		TLELine1:    l1,
		TLELine2:    l2,
	}
}
