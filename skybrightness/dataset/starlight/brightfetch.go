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
	// the Bright Star Catalogue, ten arcseconds.
	//
	// Twice the Gaia radius rather than six times, because the two matches
	// answer different questions. That one decides whether Gaia saw a star at
	// all, where a false positive silently drops it from the map; this one
	// only attaches a colour index, and the catalogue holds about 9,100 stars
	// over the whole sky, so a window this size almost never contains two.
	//
	// It was thirty arcseconds, on the reasoning that the slack would absorb
	// the proper motion between the Hipparcos epoch and J2000 without having
	// to propagate it. That was wrong, and Alpha Centauri A is where it showed:
	// at 3.7 arcseconds a year it moves 32 in that interval and fell outside,
	// so the fourth brightest star in the sky got no R magnitude. Widening
	// further would have been worse than useless - Alpha Centauri A and B sit
	// 1.4 arcseconds apart in the catalogue, so a radius large enough to
	// recover the star is large enough to take the wrong component of the
	// binary, and picking the nearer would have been luck rather than
	// correctness. The positions are propagated now, and the radius is what a
	// position match needs rather than what an unpropagated one needs.
	BrightStarCatalogueRadius = angle.Angle(10 * math.Pi / 180 / 3600)

	// BrightStarMagnitudeTolerance is how far a candidate's V may sit from the
	// star's before it is taken to be a different object.
	//
	// Both catalogues give Johnson V for the same stars, and the two
	// populations are well separated: a genuine match agrees to a few
	// hundredths, while the three multiples in this set where Hipparcos
	// reports combined light and the Bright Star Catalogue reports a component
	// differ by 0.60, 0.62 and 0.61. Half a magnitude sits between those, wide
	// enough to survive real scatter and variability and narrow enough to
	// refuse a companion.
	BrightStarMagnitudeTolerance = 0.5
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

		star.pmRA, star.pmDec = pra, pde

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
// # What it does not match, and why that is right
//
// Of 74 stars, 66 get an R. Every gap is accounted for:
//
//   - Four have a null R-I in the Bright Star Catalogue - Acrux, Beta
//     Centauri and two others. There is no value to take.
//   - One is not in the catalogue at all: HIP 39827 at V = 6.77, past its
//     completeness near 6.5.
//   - Three are multiples where the two catalogues report different things.
//     Hipparcos gives the combined light of Gamma Leonis, Xi Ursae Majoris
//     and Xi Scorpii while the Bright Star Catalogue lists their components
//     separately, so the candidate carrying R-I is fainter by 0.60, 0.62 and
//     0.61 magnitudes. That is not measurement scatter, it is the difference
//     between a pair and one of its stars, and attaching one component's
//     colour to the pair's magnitude would be inventing a number rather than
//     reading one.
//
// A star with no match keeps no R entry, and [AddBrightStars] then contributes
// it to every other band and not to R - so the R map is short by exactly those
// stars rather than carrying colours that belong to something else.
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
	adql := fmt.Sprintf("SELECT RAJ2000, DEJ2000, Vmag, %cR-I%c FROM %cV/50/catalog%c "+
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
		vmag    float64
		ri      float64
	}

	catalogue := make([]entry, 0, len(rows))

	for _, row := range rows {
		ra, okRA := numField(index, row, "raj2000")
		dec, okDec := numField(index, row, "dej2000")

		vmag, okV := numField(index, row, "vmag")

		ri, okRI := numField(index, row, "r-i")
		if !okRA || !okDec || !okRI || !okV {
			continue
		}

		catalogue = append(catalogue, entry{angle.Deg(ra), angle.Deg(dec), vmag, ri})
	}

	if len(catalogue) == 0 {
		return 0, fmt.Errorf("%w: the bright star catalogue returned no usable rows",
			ErrBrightStar)
	}

	// Hipparcos catalogues at J1991.25 and the Bright Star Catalogue gives
	// J2000 positions, so a star has moved by eight and three quarter years of
	// proper motion between them.
	const epochGap = 2000.0 - 1991.25

	for i := range stars {
		s := &stars[i]

		if !s.hasVminusI {
			continue
		}

		ra, dec := propagate(s.RA, s.Dec, s.pmRA, s.pmDec, epochGap)
		here := coord.NewICRS(ra, dec)

		// Brightness decides, not distance. Both are needed: position finds
		// the candidates and magnitude says which of them is this star.
		//
		// Alpha Centauri is why. Its two components sit 1.4 arcseconds apart
		// in the catalogue while the residual after propagating a position
		// through eight and three quarter years is about six - the catalogue
		// quantises its coordinates, and the pair orbits each other besides -
		// so the nearer of the two was B by two hundredths of an arcsecond.
		// A coin flip, and it landed on the wrong star: R-I of 0.30 rather
		// than 0.22, which is the fourth brightest star in the sky given the
		// colour of its companion.
		//
		// V separates them without ambiguity, -0.01 against 1.33, and it is a
		// property of the star rather than of how well two catalogues agree
		// about where it is.
		best, bestDelta := -1, math.Inf(1)
		bestSep := angle.Angle(math.Inf(1))

		for j, c := range catalogue {
			d := coord.Separation(here, coord.NewICRS(c.ra, c.dec))
			if d > BrightStarCatalogueRadius {
				continue
			}

			delta := math.Abs(c.vmag - s.Vmag)
			if delta < bestDelta || (delta == bestDelta && d < bestSep) {
				best, bestDelta, bestSep = j, delta, d
			}
		}

		// A candidate whose brightness does not agree is a neighbour, not this
		// star. Half a magnitude is far wider than the two catalogues disagree
		// for the same object and far narrower than the gap between a star and
		// anything else close enough to be a candidate.
		if best < 0 || bestDelta > BrightStarMagnitudeTolerance {
			continue
		}

		s.Mag["R"] = s.Vmag - s.vMinusI + catalogue[best].ri
		matched++
	}

	return matched, nil
}
