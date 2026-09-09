package plan

import (
	"bufio"
	"cmp"
	"context"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/vector"
)

// MPCObservatory is one row of the IAU Minor Planet Center's observatory-code
// list — the ~2,700-entry register every asteroid and comet observation is
// reported against.
//
// # Accuracy, which is not uniform and not what a reader expects
//
// The MPC does not publish latitude and height. It publishes the longitude and
// the two parallax constants an observation reduction actually needs, and the
// geodetic position here is recovered from them (see [MPCObservatories] for
// how). Recovering it is exact. The input is not, and it is not equally
// imprecise from row to row — which is the part worth knowing, because nothing
// in a recovered latitude and height says which kind of row it came from.
//
// The constants are published to between three and six decimals, and a unit in
// the last place scales straight into height. Counted over the 2,692 positioned
// rows on 2026-09-08:
//
//	decimals   rows   height is good to
//	       6   1322   ±3 m
//	       5   1242   ±32 m
//	       4     90   ±319 m
//	       3     38   ±3.2 km
//
// [MPCObservatory.ResolutionM] carries that per row, so a caller can tell the
// two apart instead of trusting all 2,692 equally. The 38 coarsest rows are
// where it stops being academic: 507 Nyenheim recovers to −6,655 m, which is
// not a mistake in this code or in the MPC's — it is a site near sea level
// reported to three decimals, and −6.6 km is inside that row's own ±3.2 km at
// two units in the last place.
//
// Against this package's hand-curated sites, whose coordinates come from each
// observatory's own published values, the six-decimal rows agree to 0.04″ in
// latitude and 2 m in height. The disagreements above that are real and are not
// this code's: they are sites where the MPC codes one specific telescope and
// the curated entry names the observatory.
//
// So: right for reducing an observation and right for pointing a telescope,
// wrong for anything that needs the metre, and not to be trusted at all below
// five decimals without checking ResolutionM. A site you own should be entered
// with your own surveyed coordinates through [NewSite].
type MPCObservatory struct {
	// Location is the recovered geodetic position, or nil when the MPC
	// publishes no parallax constants for this code.
	//
	// Thirty entries have none, and they are not errors or gaps in the data:
	// they are the space telescopes (Hubble, Gaia, JWST, SOHO, Spitzer) and
	// the roving-observer placeholders that stand in for an observer who
	// reports their own position with each observation. A code like "247" is
	// a perfectly valid MPC code that no [Site] can be built from, which is
	// why this is a nil position rather than a missing row.
	//
	// Three further rows — 244, 248 and 500 — publish constants that are
	// exactly zero, which is the geocentre and recovers to 6,378 km below
	// the ellipsoid. That is what the file says and it is reported as given:
	// deciding that the centre of the Earth is not a place would be this
	// package inventing a policy the MPC did not write down.
	Location *coord.Geodetic

	// ResolutionM is how finely this row was published, in metres: half a
	// unit in the last decimal of its parallax constants, scaled by Earth's
	// equatorial radius. Zero for a row with no constants.
	//
	// It is a floor on the error, not the error. A row published to six
	// decimals can still name a different pier than the one you meant; a row
	// published to three cannot be better than ±3.2 km however well the site
	// is surveyed. Read it before treating a recovered height as a height.
	ResolutionM float64

	// Code is the MPC observatory code: three characters, either digits
	// ("568") or a letter and two digits ("G37").
	Code string

	// Name is the MPC's own designation for the site, verbatim, including
	// the town it disambiguates with — "European Southern Observatory,
	// La Silla".
	Name string
}

// mpcCache holds the parsed list for the life of the process.
//
// The mutex is held across the fetch so concurrent first calls make one
// request between them rather than one each, and a failure is not recorded:
// a load that fails because consent had not been granted, or because the MPC
// was briefly unreachable, must not make the whole register permanently absent
// while the process looks healthy. The next call tries again.
var mpcCache struct {
	mu   sync.Mutex
	list []MPCObservatory
}

// MPCObservatories returns every entry in the IAU Minor Planet Center's
// observatory-code list, sorted by code.
//
// The list — about 200 KB — is fetched on the first call that needs it, using
// that caller's context, and kept for the life of the process. It is a
// [remote.Downloadable] endpoint, so a caller must have granted
// [remote.EnableDownloads] for [remote.MPCObsCodes] first; without it the error
// is [remote.ErrDownloadDenied] and says which call grants it.
//
// # Recovering a position from the parallax constants
//
// Each row gives the east longitude λ and the two parallax constants
// ρcosφ′ and ρsinφ′, in units of Earth's equatorial radius. Those two are the
// cylindrical coordinates of the site in the terrestrial frame — distance from
// the rotation axis and distance from the equatorial plane — so multiplying by
// the equatorial radius and applying λ gives a Cartesian ECEF position, which
// [coord.FromECEF] turns into geodetic longitude, latitude and height. No
// iteration and no approximation of our own: the only thing being assumed is
// which radius the constants are scaled by.
//
// The MPC's own reductions use 6378.140 km (IAU 1976) where this uses WGS 84's
// 6378.137 km. The 3 m difference in the scale factor moves a recovered height
// by about 3 m and a latitude not at all, which is two orders of magnitude
// below the quantisation of the published constants — see [MPCObservatory] for
// what the accuracy actually is.
func MPCObservatories(ctx context.Context) ([]MPCObservatory, error) {
	mpcCache.mu.Lock()
	defer mpcCache.mu.Unlock()

	if mpcCache.list != nil {
		return mpcCache.list, nil
	}

	bucket, key, err := remote.GetFile(ctx, remote.MPCObsCodes, mpcObsCodesFile)
	if err != nil {
		return nil, fmt.Errorf("plan: fetch MPC observatory codes: %w", err)
	}

	r, err := bucket.NewReader(ctx, key, nil)
	if err != nil {
		return nil, fmt.Errorf("plan: open MPC observatory codes: %w", err)
	}

	defer r.Close() //nolint:errcheck // read-only handle, nothing actionable on close failure

	list, err := parseMPCObsCodes(r)
	if err != nil {
		return nil, fmt.Errorf("plan: parse MPC observatory codes: %w", err)
	}

	mpcCache.list = list

	return list, nil
}

// NewMPCSite resolves an IAU Minor Planet Center observatory code to a [Site],
// fetching the code list on the first call that needs it (see
// [MPCObservatories] for the fetch, the consent gate, and how much the recovered
// position is worth).
//
// The code is matched case-insensitively after trimming spaces, so "g37",
// "G37" and " G37 " are the same site. The returned Site carries the MPC's own
// name and the code itself as its [Site.MPCCode], so a site built this way
// always says where it came from.
//
// Three outcomes, deliberately distinct:
//
//   - The code names a site with a position: a *Site.
//   - The code is not in the list: [ErrUnknownSite].
//   - The code is in the list but has no ground position — a space telescope,
//     the geocentre, a roving observer: [ErrSiteNotOnEarth]. Reporting that as
//     "unknown" would tell a caller their code was wrong when it is valid and
//     the answer is that no such site exists.
//
// Time zone is not set: the MPC list has no zone column, and guessing one from
// a longitude is wrong across most of the world. Chain [Site.WithTimeZone] on
// the result when local time matters.
func NewMPCSite(ctx context.Context, code string) (*Site, error) {
	list, err := MPCObservatories(ctx)
	if err != nil {
		return nil, err
	}

	return lookupMPCSite(list, code)
}

// lookupMPCSite is NewMPCSite without the fetch, so the three outcomes can be
// tested without a network and without seeding a package-level cache.
func lookupMPCSite(list []MPCObservatory, code string) (*Site, error) {
	want := strings.ToUpper(strings.TrimSpace(code))

	for _, obs := range list {
		if obs.Code != want {
			continue
		}

		if obs.Location == nil {
			return nil, fmt.Errorf("%w: %q (%s)", ErrSiteNotOnEarth, code, obs.Name)
		}

		return NewSite(obs.Name, obs.Location, WithMPCCode(obs.Code))
	}

	return nil, fmt.Errorf("%w: MPC code %q", ErrUnknownSite, code)
}

// mpcObsCodesFile is the object name under remote.MPCObsCodes' URL. The .html
// suffix is the MPC's filename, not a description: the body is one <pre>
// element wrapping a fixed-width table.
const mpcObsCodesFile = "ObsCodes.html"

// MPC column offsets. The fields are fixed-width and run together with no
// separator whenever a value fills its field — row 005 reads
// "005   2.231000.659891+0.748875Meudon" — so this is parsed by column and
// never by splitting on whitespace, which silently drops every such row.
const (
	mpcCodeEnd = 3  // [0:3]   code
	mpcLonEnd  = 13 // [3:13]  east longitude, degrees
	mpcCosEnd  = 21 // [13:21] rho cos phi'
	mpcSinEnd  = 30 // [21:30] rho sin phi', signed
)

// parseMPCObsCodes reads the MPC's fixed-width observatory table.
//
// It is deliberately tolerant of the wrapper and strict about the rows: the
// <pre> tags and the column header are skipped, a row with blank parallax
// constants becomes an entry with a nil Location, and a row whose numbers do
// not parse is an error rather than a silently dropped observatory. A list
// quietly missing the site a caller asked for is the failure mode worth
// avoiding here, since the caller would read it as "no such code".
func parseMPCObsCodes(r io.Reader) ([]MPCObservatory, error) {
	var (
		out  []MPCObservatory
		line int
	)

	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line++

		row := strings.TrimRight(sc.Text(), "\r\n")

		switch {
		case len(row) < mpcSinEnd:
			// Blank lines, the <pre> tags, and anything else too short to
			// carry a row. Every real row reaches at least the name column.
			continue
		case strings.HasPrefix(row, "Code"):
			continue
		case !looksLikeMPCCode(row[:mpcCodeEnd]):
			// Markup, not data — an error page served with a 200, or
			// whatever the MPC decides to wrap the table in next. Told apart
			// from a data row by the code field alone, and by what a code
			// cannot contain rather than by what today's codes look like:
			// every one of the 2,722 is [0-9A-Z][0-9][0-9], but pinning that
			// shape would silently drop the first code issued outside it,
			// which is the failure this parser is most careful to avoid.
			continue
		}

		obs, err := parseMPCRow(row)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}

		out = append(out, obs)
	}

	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	if len(out) == 0 {
		// An empty list and a successful parse of an empty document look the
		// same to a caller, and the second is what a truncated or redirected
		// fetch produces.
		return nil, ErrNoMPCObservatories
	}

	slices.SortFunc(out, func(a, b MPCObservatory) int { return cmp.Compare(a.Code, b.Code) })

	return out, nil
}

// looksLikeMPCCode reports whether the three-character code field could hold a
// code at all: three alphanumerics, no spaces, no punctuation.
func looksLikeMPCCode(field string) bool {
	if len(field) != mpcCodeEnd {
		return false
	}

	for _, r := range field {
		isDigit := r >= '0' && r <= '9'
		isUpper := r >= 'A' && r <= 'Z'
		isLower := r >= 'a' && r <= 'z'

		if !isDigit && !isUpper && !isLower {
			return false
		}
	}

	return true
}

// parseMPCRow converts one fixed-width row. A row with no parallax constants
// yields an entry with a nil Location rather than an error — see
// MPCObservatory.Location for what those rows are.
func parseMPCRow(row string) (MPCObservatory, error) {
	obs := MPCObservatory{
		Code: strings.TrimSpace(row[:mpcCodeEnd]),
		Name: strings.TrimSpace(row[mpcSinEnd:]),
	}

	var (
		lonText = strings.TrimSpace(row[mpcCodeEnd:mpcLonEnd])
		cosText = strings.TrimSpace(row[mpcLonEnd:mpcCosEnd])
		sinText = strings.TrimSpace(row[mpcCosEnd:mpcSinEnd])
	)

	if lonText == "" || cosText == "" || sinText == "" {
		return obs, nil
	}

	lonDeg, err := strconv.ParseFloat(lonText, 64)
	if err != nil {
		return MPCObservatory{}, fmt.Errorf("%w: %s: longitude %q", ErrMalformedMPCRow, obs.Code, lonText)
	}

	rhoCos, err := strconv.ParseFloat(cosText, 64)
	if err != nil {
		return MPCObservatory{}, fmt.Errorf("%w: %s: rho cos phi' %q", ErrMalformedMPCRow, obs.Code, cosText)
	}

	rhoSin, err := strconv.ParseFloat(sinText, 64)
	if err != nil {
		return MPCObservatory{}, fmt.Errorf("%w: %s: rho sin phi' %q", ErrMalformedMPCRow, obs.Code, sinText)
	}

	loc, err := mpcGeodetic(lonDeg, rhoCos, rhoSin)
	if err != nil {
		return MPCObservatory{}, fmt.Errorf("%s: %w", obs.Code, err)
	}

	obs.Location = loc
	obs.ResolutionM = mpcResolution(cosText, sinText)

	return obs, nil
}

// mpcResolution converts how many decimals a row's parallax constants carry
// into metres on the ground: half a unit in the last place, scaled by the
// equatorial radius. The coarser of the two constants decides, since a
// position is no better than its worse component.
func mpcResolution(cosText, sinText string) float64 {
	decimals := min(mpcDecimals(cosText), mpcDecimals(sinText))

	return 0.5 * math.Pow(10, -float64(decimals)) * constants.WGS84.SemiMajorAxis.Value
}

// mpcDecimals counts the digits after the decimal point in a published
// constant. A value written without one is worth no more than a whole Earth
// radius, which is what zero decimals correctly reports.
func mpcDecimals(text string) int {
	dot := strings.IndexByte(text, '.')
	if dot < 0 {
		return 0
	}

	return len(text) - dot - 1
}

// mpcGeodetic recovers a geodetic position from an east longitude in degrees
// and the two parallax constants. See MPCObservatories' doc comment for why
// this is a coordinate change rather than an inversion.
func mpcGeodetic(lonDeg, rhoCos, rhoSin float64) (*coord.Geodetic, error) {
	a := constants.WGS84.SemiMajorAxis.Value
	lam := angle.Deg(lonDeg).Radians()

	ecef := vector.V3(
		a*rhoCos*math.Cos(lam),
		a*rhoCos*math.Sin(lam),
		a*rhoSin,
	)

	loc, err := coord.FromECEF(ecef, coord.WGS84())
	if err != nil {
		return nil, fmt.Errorf("recover position: %w", err)
	}

	return loc, nil
}
