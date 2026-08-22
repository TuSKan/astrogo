package starlight

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/remote/api"
)

// Defaults for [FetchBrightStars], measured rather than chosen.
const (
	// BrightStarLimitV is how faint to look for stars Gaia might be missing.
	//
	// Seventy of the 74 stars actually missing are brighter than V = 3 and the
	// faintest is 6.77, so V = 7 is already past where the population ends.
	// Going deeper costs a much larger Hipparcos query and finds nothing.
	BrightStarLimitV = 7.0

	// BrightStarMatchRadius is the positional tolerance for deciding that a
	// Hipparcos star already has a Gaia counterpart.
	//
	// The answer barely depends on it: 81 stars are unmatched at 2 arcsec, 74
	// at 5, 66 at 10 and 64 at 20. Five is the middle of that plateau.
	//
	// Spelled as a constant expression rather than angle.Arcsec(5) because
	// [angle.Angle] is radians and its constructors are functions, so the
	// alternative is an exported variable somebody could reassign.
	BrightStarMatchRadius = angle.Angle(5 * math.Pi / 180 / 3600)

	// BrightStarCatalogueRadius is the tolerance for matching a bright star to
	// the Bright Star Catalogue, thirty arcseconds.
	//
	// Six times the Gaia radius, and it can afford to be. That match decides
	// whether Gaia saw a star at all, where a false positive silently drops a
	// star from the map; this one only attaches a colour index, the catalogue
	// holds about 9,100 stars over the whole sky — one per four square
	// degrees — so a window this size almost never contains two, and the
	// nearest is unambiguous. The slack also absorbs the eight and three
	// quarter years of proper motion between the Hipparcos epoch and J2000
	// without propagating it: over that span even Sirius moves about eleven
	// arcseconds.
	BrightStarCatalogueRadius = angle.Angle(30 * math.Pi / 180 / 3600)
)

// FetchBrightStars returns the Hipparcos stars Gaia has no counterpart for,
// ready to hand to [AddBrightStars].
//
// # Why this is not a crossmatch-table lookup
//
// The Gaia archive publishes gaiadr3.hipparcos2_best_neighbour, and the obvious
// implementation is to take the Hipparcos stars missing from it. That is wrong
// by a factor of 250: 18,693 Hipparcos stars have no row there, but a missing
// crossmatch row means the crossmatch failed — close pairs, high proper motion
// — not that Gaia never saw the star. Adding those would double-count nearly
// all of them. Only a positional check answers the question that matters, which
// is whether the light is already in the map, so that is what this does.
//
// Hipparcos is catalogued at J1991.25 and Gaia DR3 at J2016.0, so positions are
// propagated by proper motion first; see [BrightStarsMissingFromGaia].
//
// Two queries, one to VizieR for the Hipparcos photometry and one to the Gaia
// archive for everything bright enough to be a counterpart. That is ordinary
// use of both services, unlike [BuildFromGaia].
func FetchBrightStars(ctx context.Context, faintestV float64, radius angle.Angle) ([]BrightStar, error) {
	if math.IsNaN(faintestV) || faintestV <= -2 {
		return nil, fmt.Errorf("%w: faintest V = %v", ErrBrightStar, faintestV)
	}

	if radius <= 0 {
		return nil, fmt.Errorf("%w: match radius = %v", ErrBrightStar, radius)
	}

	stars, pmRA, pmDec, err := fetchHipparcos(ctx, faintestV)
	if err != nil {
		return nil, err
	}

	// A Hipparcos star at the V limit can sit near G = V + 2 if it is very red,
	// so the Gaia side reaches deeper than the Hipparcos side. Otherwise a
	// genuine counterpart falls outside the query, its star is reported
	// missing, and adding it would count that light twice.
	gaiaRA, gaiaDec, err := fetchGaiaBright(ctx, faintestV+2)
	if err != nil {
		return nil, err
	}

	return BrightStarsMissingFromGaia(stars, pmRA, pmDec, gaiaRA, gaiaDec, radius)
}

// fetchHipparcos reads the original Hipparcos catalogue from VizieR.
//
// I/239/hip_main, not I/311/hip2. The latter is van Leeuwen (2007), an
// astrometric reduction carrying Hpmag, B-V and V-I but no Johnson V — and
// Johnson V is the whole point, since it is used directly with no colour
// transformation, which is what makes these stars immune to the transformation
// error this package carried for so long.
func fetchHipparcos(ctx context.Context, faintestV float64) ([]BrightStar, []float64, []float64, error) {
	client, err := api.NewClient(remote.VizieR,
		api.WithTimeout(3*time.Minute),
		api.WithMinInterval(aggregationPace))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("starlight: vizier client: %w", err)
	}

	defer func() { _ = client.Close() }()

	// B-V and V-I come back with the photometry, which is what makes a
	// multi-band map possible without a colour fit: B = V + (B-V) and
	// I = V - (V-I), both exact.
	// No HD: the VizieR view of this table does not carry one, which the
	// service reports as an unresolved identifier and which is why the Bright
	// Star Catalogue is matched by position rather than joined by number.
	adql := fmt.Sprintf("SELECT HIP, RAICRS, DEICRS, Vmag, %cB-V%c, %cV-I%c, "+
		"pmRA, pmDE, RAhms, DEdms "+
		"FROM %cI/239/hip_main%c WHERE Vmag < %g",
		'"', '"', '"', '"', '"', '"', faintestV)

	body, err := client.PostForm(ctx, remote.VizieR, "", tapForm(adql))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("starlight: hipparcos query: %w", err)
	}

	defer func() { _ = body.Close() }()

	return parseHipparcos(body)
}

// parseHipparcos reads the VizieR CSV into stars and proper motions.
func parseHipparcos(r io.Reader) ([]BrightStar, []float64, []float64, error) {
	rows, index, err := readTAPCSV(r)
	if err != nil {
		return nil, nil, nil, err
	}

	var (
		stars       []BrightStar
		pmRA, pmDec []float64
	)

	for _, row := range rows {
		v, ok := numField(index, row, "vmag")
		if !ok {
			continue
		}

		raA, decA, ok := hipparcosPosition(index, row)
		if !ok {
			continue
		}

		hip, _ := numField(index, row, "hip")
		pra, _ := numField(index, row, "pmra")
		pde, _ := numField(index, row, "pmde")

		star := BrightStar{
			HIP:  int(hip),
			RA:   raA,
			Dec:  decA,
			Vmag: v,
			Mag:  map[string]float64{"V": v},
		}

		// B and I follow from the catalogue's own colour indices. A star
		// missing one simply has no magnitude in that band; nothing is
		// interpolated to fill it.
		if bv, ok := numField(index, row, "b-v"); ok {
			star.Mag["B"] = v + bv
		}

		if vi, ok := numField(index, row, "v-i"); ok {
			star.Mag["I"] = v - vi
			star.vMinusI, star.hasVminusI = vi, true
		}

		stars = append(stars, star)
		pmRA = append(pmRA, pra)
		pmDec = append(pmDec, pde)
	}

	if len(stars) == 0 {
		return nil, nil, nil, fmt.Errorf("%w: no Hipparcos rows", ErrBrightStar)
	}

	return stars, pmRA, pmDec, nil
}

// hipparcosPosition prefers the ICRS columns and falls back to the sexagesimal
// ones.
//
// 262 entries carry no ICRS position because the astrometric fit failed on
// them, and three of those are naked-eye stars. The sexagesimal columns still
// hold a position, and skipping them would drop exactly the objects this
// correction exists for.
func hipparcosPosition(index map[string]int, row []string) (ra, dec angle.Angle, ok bool) {
	a, okRA := numField(index, row, "raicrs")
	d, okDec := numField(index, row, "deicrs")

	if okRA && okDec {
		return angle.Deg(a), angle.Deg(d), true
	}

	ra, okRA = sexagesimal(textField(index, row, "rahms"), true)
	dec, okDec = sexagesimal(textField(index, row, "dedms"), false)

	return ra, dec, okRA && okDec
}

// fetchGaiaBright reads every Gaia source bright enough to be a counterpart.
//
// A source with no flux is excluded: it exists in the catalogue but contributes
// nothing to the map, so for the question being asked — is this star's light
// already counted? — it is not a counterpart.
func fetchGaiaBright(ctx context.Context, faintestG float64) ([]angle.Angle, []angle.Angle, error) {
	client, err := aggregationClient(remote.GaiaTAP)
	if err != nil {
		return nil, nil, err
	}

	defer func() { _ = client.Close() }()

	adql := fmt.Sprintf("SELECT ra, dec FROM gaiadr3.gaia_source "+
		"WHERE phot_g_mean_mag < %g AND phot_g_mean_flux > 0", faintestG)

	body, err := client.PostForm(ctx, remote.GaiaTAP, "", tapForm(adql))
	if err != nil {
		return nil, nil, fmt.Errorf("starlight: bright gaia query: %w", err)
	}

	defer func() { _ = body.Close() }()

	rows, index, err := readTAPCSV(body)
	if err != nil {
		return nil, nil, err
	}

	ra := make([]angle.Angle, 0, len(rows))
	dec := make([]angle.Angle, 0, len(rows))

	for _, row := range rows {
		a, okA := numField(index, row, "ra")
		d, okD := numField(index, row, "dec")

		if !okA || !okD {
			continue
		}

		ra = append(ra, angle.Deg(a))
		dec = append(dec, angle.Deg(d))
	}

	if len(ra) == 0 {
		return nil, nil, fmt.Errorf("%w: no Gaia rows", ErrBrightStar)
	}

	return ra, dec, nil
}

// tapForm builds the POST body every IVOA TAP sync endpoint takes.
func tapForm(adql string) url.Values {
	v := url.Values{}
	v.Set("REQUEST", "doQuery")
	v.Set("LANG", "ADQL")
	v.Set("FORMAT", "csv")
	v.Set("QUERY", adql)

	return v
}

// readTAPCSV reads a whole TAP CSV response into rows plus a lowercased header
// index. Lowercasing is what lets one reader serve both archives: ESA
// lowercases its column names and VizieR preserves the case it was given.
func readTAPCSV(r io.Reader) ([][]string, map[string]int, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrGaiaResponse, err)
	}

	index := make(map[string]int, len(header))
	for i, name := range header {
		index[strings.ToLower(strings.TrimSpace(name))] = i
	}

	rows, err := reader.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrGaiaResponse, err)
	}

	return rows, index, nil
}

// numField reads one numeric column.
func numField(index map[string]int, row []string, name string) (float64, bool) {
	s := textField(index, row, name)
	if s == "" {
		return 0, false
	}

	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) {
		return 0, false
	}

	return v, true
}

// textField reads one column as a trimmed string.
func textField(index map[string]int, row []string, name string) string {
	i, ok := index[name]
	if !ok || i >= len(row) {
		return ""
	}

	return strings.TrimSpace(row[i])
}

// sexagesimal parses "HH MM SS.SS" or "+DD MM SS.S".
func sexagesimal(s string, isHours bool) (angle.Angle, bool) {
	parts := strings.Fields(s)
	if len(parts) != 3 {
		return 0, false
	}

	neg := strings.HasPrefix(parts[0], "-")

	var v [3]float64

	for i := range 3 {
		x, err := strconv.ParseFloat(strings.TrimLeft(parts[i], "+-"), 64)
		if err != nil {
			return 0, false
		}

		v[i] = x
	}

	deg := v[0] + v[1]/60 + v[2]/3600
	if isHours {
		deg *= 15
	}

	if neg {
		deg = -deg
	}

	return angle.Deg(deg), true
}

// AddCousinsR fills in each star's R magnitude from the Bright Star Catalogue,
// returning how many it matched.
//
// # Why this is a separate step
//
// It is the one band that needs a second catalogue, and [FetchBrightStars]
// does not call it. A caller wanting only V, B and I should not have their
// fetch fail because a service they do not need is down, and a caller wanting
// R should see that failure rather than get a map quietly missing its
// brightest stars in one band.
//
// # Why a second catalogue at all
//
// Hipparcos publishes V, B-V and V-I but no R and no V-R, so R is the one
// Johnson-Cousins band its photometry cannot reach. The Bright Star Catalogue
// (Hoffleit & Jaschek 1991, VizieR V/50) publishes R-I for the same stars, and
// the two together give R exactly:
//
//	V-R = (V-I) - (R-I)
//	R   = V - (V-I) + (R-I)
//
// Nothing is fitted. The alternative would be a colour-colour relation
// predicting V-R from B-V, which is a regression over somebody's sample, and
// putting an estimate into a map that is otherwise measurements — for the
// brightest stars in the sky, where an error is least forgivable — is what
// this package refuses everywhere else.
//
// # The match
//
// By position, because the VizieR view of I/239/hip_main carries no HD number
// to join on. The radius is generous and can afford to be: the Bright Star
// Catalogue holds about 9,100 stars over the whole sky, one per four square
// degrees, so a window this size almost never contains two and the nearest is
// unambiguous. That also absorbs the eight and three quarter years of proper
// motion between the Hipparcos epoch and J2000 without propagating it — over
// which even Sirius, among the fastest bright stars, moves about eleven
// arcseconds.
//
// A star the catalogue does not cover keeps no R entry, and [AddBrightStars]
// then contributes it to every other band and not to R.
func AddCousinsR(ctx context.Context, stars []BrightStar) (matched int, err error) {
	var wanted int

	for _, s := range stars {
		if s.hasVminusI {
			wanted++
		}
	}

	if wanted == 0 {
		return 0, nil
	}

	client, err := api.NewClient(remote.VizieR,
		api.WithTimeout(aggregationTimeout),
		api.WithMinInterval(aggregationPace))
	if err != nil {
		return 0, fmt.Errorf("starlight: vizier client: %w", err)
	}

	defer func() { _ = client.Close() }()

	// The whole catalogue is nine thousand rows of three columns. The
	// alternative is a positional disjunction over every star we hold, which
	// is a longer query than the answer.
	adql := fmt.Sprintf("SELECT RAJ2000, DEJ2000, %cR-I%c FROM %cV/50/catalog%c "+
		"WHERE %cR-I%c IS NOT NULL", '"', '"', '"', '"', '"', '"')

	body, err := client.PostForm(ctx, remote.VizieR, "", tapForm(adql))
	if err != nil {
		return 0, fmt.Errorf("starlight: bright star catalogue query: %w", err)
	}

	defer func() { _ = body.Close() }()

	rows, index, err := readTAPCSV(body)
	if err != nil {
		return 0, err
	}

	type entry struct {
		ra, dec angle.Angle
		ri      float64
	}

	catalogue := make([]entry, 0, len(rows))

	for _, row := range rows {
		ra, okRA := numField(index, row, "raj2000")
		dec, okDec := numField(index, row, "dej2000")

		ri, okRI := numField(index, row, "r-i")
		if !okRA || !okDec || !okRI {
			continue
		}

		catalogue = append(catalogue, entry{angle.Deg(ra), angle.Deg(dec), ri})
	}

	if len(catalogue) == 0 {
		return 0, fmt.Errorf("%w: the bright star catalogue returned no usable rows",
			ErrBrightStar)
	}

	for i := range stars {
		s := &stars[i]

		if !s.hasVminusI {
			continue
		}

		here := coord.NewICRS(s.RA, s.Dec)
		best, found := -1, angle.Angle(math.Inf(1))

		for j, c := range catalogue {
			if d := coord.Separation(here, coord.NewICRS(c.ra, c.dec)); d < found {
				best, found = j, d
			}
		}

		if best < 0 || found > BrightStarCatalogueRadius {
			continue
		}

		s.Mag["R"] = s.Vmag - s.vMinusI + catalogue[best].ri
		matched++
	}

	return matched, nil
}
