package sbdb

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/ephemeris/kepler"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// ErrAPIError indicates a SBDB API error response.
var ErrAPIError = errors.New("sbdb: API error")

// Provider implements resolve.Provider and resolve.ObjectResolver for SBDB.
type Provider struct {
	client *remote.Client
	cache  resolve.Cache
}

// New creates a new SBDB catalog provider.
func New() *Provider {
	client := remote.Default()

	return &Provider{
		client: client,
		cache:  resolve.NewMapCache(),
	}
}

// Name returns the provider identifier.
func (p *Provider) Name() string {
	return "sbdb"
}

// Capabilities returns the set of supported resolution operations.
func (p *Provider) Capabilities() []resolve.Capability {
	return []resolve.Capability{resolve.CapObjectResolution, resolve.CapMagnitudeBrowse}
}

// Resolve returns the object SBDB identifies by query. A query that matches
// nothing is [resolve.ErrNotFound]. One that several objects match is
// [resolve.ErrAmbiguous], unless exactly one of them has query as its
// primary designation, as "73P" is the parent of 73P-A and the other
// fragments; that one is returned.
func (p *Provider) Resolve(ctx context.Context, query string) (resolve.Target, error) {
	targets, err := p.Search(ctx, query)
	if err != nil {
		return resolve.Target{}, err
	}

	if len(targets) == 0 {
		return resolve.Target{}, fmt.Errorf("%w: %q in sbdb", resolve.ErrNotFound, query)
	}

	return targets[0], nil
}

// Search performs fuzzy search across all MPC-registered small bodies.
func (p *Provider) Search(ctx context.Context, query string) ([]resolve.Target, error) {
	req := resolve.ObjectRequest{Query: query, Limit: 1}

	targets, err := resolve.Drain(p.ResolveObject(ctx, req), 1)
	if err != nil {
		return nil, fmt.Errorf("searching for %q: %w", query, err)
	}

	return targets, nil
}

// ResolveObject performs streaming resolution via the JPL Small-Body Database API.
func (p *Provider) ResolveObject(ctx context.Context, req resolve.ObjectRequest) resolve.SeqIterator[resolve.Target] {
	queryKey := resolve.Normalize(req.Query)
	cacheKey := "resolve:sbdb:" + queryKey

	if seq, ok := p.cache.Get(cacheKey); ok {
		return seq
	}

	return func(yield func(resolve.Target, error) bool) {
		payload, err := p.identify(ctx, req.Query)
		if err != nil {
			yield(resolve.Target{}, err)
			return
		}

		// payload.Object.Kind is a 2-char code (confirmed live: "an"/"au" for
		// numbered/unnumbered asteroids, "cn"/"cu" for numbered/unnumbered
		// comets) — a strict "c" equality here would never match any real
		// response; checking the leading letter is the correct test.
		isComet := strings.HasPrefix(payload.Object.Kind, "c")

		// SBDB's own real "orbit.elements" array (verified live, not
		// guessed) publishes e, a (au), i/om/w/ma (deg) natively in the
		// units resolve.Target's own orbital-element fields expect — no
		// unit conversion needed, unlike Horizons' ELEMENTS ephemeris
		// type (which defaults to km-seconds).
		var (
			eccentricity                           float64
			semiMajorAxis, incl, node, argp, ma    float64
			hasA, hasIncl, hasNode, hasArgp, hasMA bool
			q, tp                                  float64
			hasQ, hasTP                            bool
		)

		for _, el := range payload.Orbit.Elements {
			switch el.Name {
			case "e":
				if v, err := parseFloat(el.Value); err == nil {
					eccentricity = v
				}
			case "a":
				if v, err := parseFloat(el.Value); err == nil {
					semiMajorAxis = v
					hasA = true
				}
			case "i":
				if v, err := parseFloat(el.Value); err == nil {
					incl = v
					hasIncl = true
				}
			case "om":
				if v, err := parseFloat(el.Value); err == nil {
					node = v
					hasNode = true
				}
			case "w":
				if v, err := parseFloat(el.Value); err == nil {
					argp = v
					hasArgp = true
				}
			case "ma":
				if v, err := parseFloat(el.Value); err == nil {
					ma = v
					hasMA = true
				}
			case "q":
				if v, err := parseFloat(el.Value); err == nil {
					q = v
					hasQ = true
				}
			case "tp":
				if v, err := parseFloat(el.Value); err == nil {
					tp = v
					hasTP = true
				}
			}
		}

		epochJD, epochErr := parseFloat(payload.Orbit.Epoch)
		hasAsteroidForm, hasCometForm := hasA && hasMA, hasQ && hasTP
		hasElements := hasIncl && hasNode && hasArgp && epochErr == nil && (hasAsteroidForm || hasCometForm)

		t := resolve.Target{
			ID:          payload.Object.SpkID,
			Name:        payload.Object.FullName,
			Designation: payload.Object.Des,
			SPKID:       payload.Object.SpkID,
			Kind:        classifyKind(payload.Object.SpkID, payload.Object.OrbitClass.Code, eccentricity, isComet),
			Catalog:     "sbdb",
			HasElements: hasElements,
		}

		if hasElements {
			t.Epoch = time.FromJD(epochJD, time.TDB)
			t.Eccentricity = eccentricity
			t.Inclination = angle.Deg(incl)
			t.AscendingNode = angle.Deg(node)
			t.ArgPeriapsis = angle.Deg(argp)
			setElementForms(&t, hasAsteroidForm, semiMajorAxis, ma, hasCometForm, q, tp)
		}

		// Parse physical parameters for magnitude computation.
		for _, pp := range payload.PhysPar {
			switch pp.Name {
			case "H":
				if v, err := parseFloat(pp.Value); err == nil {
					t.H = v
					t.HasH = true
				}
			case "G":
				if v, err := parseFloat(pp.Value); err == nil {
					t.G = v
				}
			case "M1":
				if v, err := parseFloat(pp.Value); err == nil {
					t.M1 = v
					t.HasM1 = true
				}
			case "K1":
				if v, err := parseFloat(pp.Value); err == nil {
					t.K1 = v
				}
			case "M2":
				if v, err := parseFloat(pp.Value); err == nil {
					t.M2 = v
				}
			case "K2":
				if v, err := parseFloat(pp.Value); err == nil {
					t.K2 = v
				}
			case "PC":
				// The nuclear magnitude's phase coefficient. Until #548 it
				// was dropped, so no nuclear magnitude could carry its phase
				// term.
				if v, err := parseFloat(pp.Value); err == nil {
					t.PC = v
				}
			case "diameter":
				if v, err := parseFloat(pp.Value); err == nil {
					// SBDB publishes phys_par diameters in kilometers.
					t.Diameter = unit.Km(v)
					t.HasDiameter = true
				}
			case "albedo":
				if v, err := parseFloat(pp.Value); err == nil {
					t.Albedo = v
					t.HasAlbedo = true
				}
			}
		}

		if err := p.cache.Set(cacheKey, []resolve.Target{t}); err != nil {
			yield(resolve.Target{}, err)
			return
		}

		if !yield(t, nil) {
			return
		}
	}
}

// lookupPayload is an SBDB API answer. Its documentation lists three for a
// valid query: the object's data; a list of the objects a non-unique query
// matches, under HTTP 300; and, under HTTP 200, a message with no object
// when nothing matches.
type lookupPayload struct {
	Object *struct {
		SpkID      string `json:"spkid"`
		FullName   string `json:"fullname"`
		Des        string `json:"des"`
		Kind       string `json:"kind"`
		OrbitClass struct {
			Code string `json:"code"`
		} `json:"orbit_class"`
	} `json:"object"`
	Message string        `json:"message"`
	List    []lookupMatch `json:"list"`
	PhysPar []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"phys_par"`
	Orbit struct {
		Epoch    string `json:"epoch"` // JD, TDB — confirmed live against the same convention as the elements' own "tp" (time of perihelion passage), which SBDB explicitly labels units:"TDB"
		Elements []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"elements"`
	} `json:"orbit"`
}

// lookupMatch is one entry of the list SBDB answers a non-unique query with.
type lookupMatch struct {
	PDes string `json:"pdes"`
	Name string `json:"name"`
}

// exactDesignation returns the primary designation of the one entry in
// list whose designation is query itself.
func exactDesignation(query string, list []lookupMatch) (string, bool) {
	var found []string

	for _, m := range list {
		if resolve.Normalize(m.PDes) == resolve.Normalize(query) {
			found = append(found, m.PDes)
		}
	}

	if len(found) != 1 {
		return "", false
	}

	return found[0], true
}

// ambiguous reports that query matched every object in list, naming the
// first few.
func ambiguous(query string, list []lookupMatch) error {
	const named = 5

	names := make([]string, 0, min(len(list), named))
	for _, m := range list[:min(len(list), named)] {
		names = append(names, m.Name)
	}

	more := ""
	if len(list) > named {
		more = fmt.Sprintf(" and %d more", len(list)-named)
	}

	return fmt.Errorf("%w: %q in sbdb matches %d objects: %s%s",
		resolve.ErrAmbiguous, query, len(list), strings.Join(names, "; "), more)
}

// brightnessMargin is how much fainter than the requested bound a body's
// absolute magnitude (H/M1) may be and still be a candidate.
//
// A body is seen brighter than its absolute magnitude only when r·Δ, the
// product of its distances in AU from the Sun and from the Earth, is below 1.
// How small r·Δ can get is bounded by the perihelion when that is beyond the
// Earth's aphelion (see couldReach), and not at all when it is inside; the
// margin limits how far either may take a candidate: 4.0 admits a body whose
// r·Δ falls to 10^(−0.8) ≈ 0.16 AU². This comment used to calibrate it
// against Ceres, "H=3.4, best-ever apparent ≈6.7, a ≈3.3 mag correction", but
// that correction runs the other way: Ceres is never brighter than about 6.3,
// three magnitudes fainter than its H, as every main-belt body is (#779).
//
// It deliberately does NOT chase extreme close-approach NEA brightening
// (e.g. 99942 Apophis's 2029 flyby: H=19.7 but a predicted peak apparent
// magnitude ≈3.1, a ≈16.6 mag correction): a margin that large would defeat
// the purpose of a prefilter (empirically, H<20 for asteroids alone numbers
// in the hundreds of thousands) and belongs to a fundamentally different
// query — a dedicated close-approach lookup against JPL's Close Approach
// Data API — which this library doesn't implement. This is a deliberate,
// documented scope limitation, not an oversight.
const brightnessMargin = 4.0

// dwarfPlanetSPKIDs holds the SPK-ID of each of the five IAU-recognized
// dwarf planets — SBDB otherwise reports every one of them as a generic
// numbered asteroid, indistinguishable from any other minor planet.
var dwarfPlanetSPKIDs = map[string]bool{
	"20000001": true, // 1 Ceres
	"20134340": true, // 134340 Pluto
	"20136199": true, // 136199 Eris
	"20136108": true, // 136108 Haumea
	"20136472": true, // 136472 Makemake
}

// interstellarOrbitClasses holds the JPL SBDB orbit-class codes meaning
// "the current best-fit orbit has eccentricity >= 1" — confirmed live
// against real designations: 1I/'Oumuamua reports orbit_class.code "HYA"
// ("Hyperbolic Asteroid"), 2I/Borisov reports "HYP" ("Hyperbolic Comet").
var interstellarOrbitClasses = map[string]bool{
	"HYA": true,
	"HYP": true,
}

// interstellarEccentricityThreshold is the minimum eccentricity this
// package requires, in addition to interstellarOrbitClasses, before
// classifying a body as KindInterstellar. This is NOT an IAU-defined
// distinction — it exists to fix a real false-positive confirmed live
// against the running SBDB Query API: several ordinary long-period
// comets (e.g. C/1937 C1 Whipple, e=1.0002; C/2000 A1 Montani, e=1.0040)
// carry JPL's own "HYP" orbit_class simply because their current
// best-fit osculating eccentricity sits fractionally above 1 — a known,
// expected artifact of fitting a near-parabolic Oort-cloud comet's orbit
// (small measurement/perturbation noise easily pushes e across 1.0 without
// the body actually being of interstellar origin), not evidence of an
// unbound trajectory. Confirmed interstellar objects sit far above this
// noise floor (1I/'Oumuamua e=1.2, 2I/Borisov e=3.36), so a 5% margin
// cleanly separates the two populations without excluding either.
const interstellarEccentricityThreshold = 1.05

// classifyKind returns resolve.KindInterstellar for a body confidently on
// an unbound (hyperbolic) orbit (see interstellarEccentricityThreshold),
// resolve.KindDwarfPlanet for one of the five known dwarf planets (by
// SPK-ID), resolve.KindComet for any comet, and resolve.KindAsteroid for
// every other minor planet.
func classifyKind(spkID, orbitClass string, eccentricity float64, isComet bool) resolve.Kind {
	switch {
	case interstellarOrbitClasses[orbitClass] && eccentricity > interstellarEccentricityThreshold:
		return resolve.KindInterstellar
	case isComet:
		return resolve.KindComet
	case dwarfPlanetSPKIDs[spkID]:
		return resolve.KindDwarfPlanet
	default:
		return resolve.KindAsteroid
	}
}

// sbdbQueryResponse is the column-oriented shape JPL's SBDB *query* API
// (distinct from the identify endpoint ResolveObject uses above) returns:
// Fields names each column, Data holds one row per match in the same
// column order. Cells decode as `any` rather than string — unlike the
// identify endpoint's phys_par values, the query API mixes JSON strings and
// numbers across columns (and sometimes within one, depending on whether a
// value is exact); cellString below normalizes either to text for parseFloat.
type sbdbQueryResponse struct {
	Fields []string `json:"fields"`
	Data   [][]any  `json:"data"`
}

// cellString renders one decoded JSON cell (string or float64, per
// encoding/json's default number handling) as text for parseFloat/direct
// use, regardless of which type the API returned it as.
func cellString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", x)
	}
}

// SearchBright returns the asteroids and comets SBDB tracks that could be
// seen brighter than req.MaxVMag: an absolute magnitude (H for asteroids, M1
// for comets) within brightnessMargin of it, and a perihelion near enough for
// the body to reach it at all (see couldReach), brightest (lowest H/M1)
// first. This is Stage 1 of a two-stage design — it returns identity plus
// H/G or M1/K1 only, never a real position or computed apparent magnitude,
// since that needs per-body ephemeris state the catalog layer deliberately
// doesn't depend on (Stage 2 lives in plan.VisibleTonight, which already
// imports both catalog and ephemeris).
//
// The perihelion bound does not depend on the date, so without req.At a comet
// that could be bright at perihelion is a candidate however long ago that
// was: 153P/Ikeya-Zhang, at perihelion in 2002, heads the list. With req.At,
// each comet is bounded by the nearest its orbit takes it to the Sun within
// a month of that instant (sunDistanceAround), and the comets are ranked
// among those. Asteroids are bounded by perihelion either way: their
// distances from the Sun vary far less, and SBDB holds too many to fetch.
//
// req.Limit defaults to 50 per kind (asteroids and comets are queried and
// capped separately, so up to 100 candidates total). Without a req.Limit, a
// kind with more candidates than that gives its 50 brightest followed by an
// error wrapping [resolve.ErrTruncated], never a short list that looks
// complete, and only a complete list is cached. queryBright sorts
// server-side by H/M1 ascending before this cap is applied, so the
// candidates kept are the lowest-H ones among those that could reach the
// bound. Sorted without that bound, they were not: the lowest-H asteroids are
// trans-Neptunian, and Juno, Hebe and Iris were cut at every bound (#779).
func (p *Provider) SearchBright(ctx context.Context, req resolve.BrightRequest) resolve.SeqIterator[resolve.Target] {
	cacheKey := fmt.Sprintf("bright:%f:%d", req.MaxVMag, req.Limit)
	if dated := !req.At.IsZero(); dated {
		cacheKey += ":" + strconv.FormatFloat(req.At.TT().JD(), 'f', -1, 64)
	}

	if seq, ok := p.cache.Get(cacheKey); ok {
		return seq
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}

	// Without a limit of the caller's own, the default is this provider's
	// ceiling, so one row past it is asked for per kind: its arrival is what
	// tells a cut list from a complete one, as for catalog/simbad. A cut list
	// is followed by resolve.ErrTruncated and not cached; it used to end
	// silently at 50, looking complete (#776).
	ask := limit
	if req.Limit <= 0 {
		ask = limit + 1
	}

	return func(yield func(resolve.Target, error) bool) {
		asteroids, asteroidRows, err := p.queryBright(ctx, "a", "H", req.MaxVMag, time.Time{}, ask)
		if err != nil {
			yield(resolve.Target{}, err)
			return
		}

		// A dated search ranks the comets that can be bright on the date,
		// which SBDB cannot select, so it is asked for every comet the
		// undated bound admits and the date is applied to each row here. It
		// holds about two thousand comets with a total magnitude, so that is
		// one answer of a few hundred kilobytes.
		cometAsk := ask
		if !req.At.IsZero() {
			cometAsk = datedCometRows
		}

		comets, cometRows, err := p.queryBright(ctx, "c", "M1", req.MaxVMag, req.At, cometAsk)
		if err != nil {
			yield(resolve.Target{}, err)
			return
		}

		// Whether a kind was cut is decided by the rows SBDB sent, not the
		// ones kept: a row past the limit means there were more, whatever
		// the exact check below then made of the rows that did arrive. A
		// dated comet search has every row SBDB holds unless it filled the
		// page, so there it is the comets kept that are counted.
		var cut []string

		if asteroidRows > limit {
			cut = append(cut, "asteroids")
		}

		cometsCut := cometRows > limit
		if !req.At.IsZero() {
			cometsCut = cometRows >= datedCometRows || (req.Limit <= 0 && len(comets) > limit)
		}

		if cometsCut {
			cut = append(cut, "comets")
		}

		asteroids = asteroids[:min(len(asteroids), limit)]
		comets = comets[:min(len(comets), limit)]

		targets := make([]resolve.Target, 0, len(asteroids)+len(comets))
		targets = append(targets, asteroids...)
		targets = append(targets, comets...)

		if len(cut) == 0 {
			if err := p.cache.Set(cacheKey, targets); err != nil {
				yield(resolve.Target{}, err)
				return
			}
		}

		for _, t := range targets {
			if !yield(t, nil) {
				return
			}
		}

		if len(cut) > 0 {
			yield(resolve.Target{}, fmt.Errorf("%w: SBDB has more than %d candidate %s for magnitude %g",
				resolve.ErrTruncated, limit, strings.Join(cut, " and "), req.MaxVMag))
		}
	}
}

// elementFields requests the same six osculating orbital elements plus
// their epoch (JD, TDB) that ResolveObject's single-object identify path
// already decodes from payload.Orbit — verified live against the real
// sbdb_query.api that it accepts these exact field names and returns
// values matching the identify endpoint's own orbit.elements (e.g. 1
// Ceres: a=2.766, e=0.0797, i=10.59, om=80.25, w=73.29, ma=274.42,
// epoch=2461200.5). Riding along on the same bulk query costs nothing
// extra (one request per sb-kind either way), so a Stage-1 bright-search
// Target carries everything ephemeris.NewFromElements needs and
// plan.FromCatalog can build a network-free Kepler-propagated body with
// no Stage-2 kernel fetch at all. "e" (eccentricity) is folded in here
// rather than requested separately — it was already fetched alone for
// classifyKind, now decoded once alongside the rest. "q" and "tp" are the
// comet form of the same orbit, which is the only form an open orbit has.
const elementFields = ",e,a,i,om,w,ma,q,tp,epoch"

// identify asks SBDB which object query names and returns its answer, which
// then always has an object.
//
// SBDB answers a query several objects match with their list, and
// documents asking again by des= to select one of them. That is done only
// when one entry's primary designation is the query itself, as "73P" is the
// parent of 73P-A, 73P-B and the rest; otherwise the answer is
// [resolve.ErrAmbiguous]. A message with no object is SBDB's documented
// answer when nothing matches ("specified object was not found"), since its
// errors arrive as HTTP error statuses instead.
func (p *Provider) identify(ctx context.Context, query string) (lookupPayload, error) {
	payload, err := p.lookup(ctx, "sstr", query)
	if err != nil {
		return lookupPayload{}, err
	}

	if len(payload.List) > 0 {
		pdes, ok := exactDesignation(query, payload.List)
		if !ok {
			return lookupPayload{}, ambiguous(query, payload.List)
		}

		if payload, err = p.lookup(ctx, "des", pdes); err != nil {
			return lookupPayload{}, err
		}

		if len(payload.List) > 0 {
			return lookupPayload{}, ambiguous(pdes, payload.List)
		}
	}

	if payload.Object != nil {
		return payload, nil
	}

	if payload.Message != "" {
		return lookupPayload{}, fmt.Errorf("%w: %q in sbdb: %s", resolve.ErrNotFound, query, payload.Message)
	}

	return lookupPayload{}, fmt.Errorf("%w: answer for %q has no object, list or message", ErrAPIError, query)
}

// lookup asks SBDB for the object key=value names, key being "sstr" (a
// search string) or "des" (a primary designation).
func (p *Provider) lookup(ctx context.Context, key, value string) (lookupPayload, error) {
	params := url.Values{}
	params.Set(key, value)
	// Request physical parameters to get H, G, M1, k1 for magnitude computation.
	params.Set("phys-par", "true")
	// Without this SBDB rounds every orbital element to three significant
	// figures — Eros comes back with a = 1.46 and e = 0.223, not 1.458243716
	// and 0.2228779628. That is not a display detail: propagating the rounded
	// elements two-body puts Eros 690,000 km from its own kernel *at the epoch
	// of osculation*, where osculating elements are exact by construction and
	// the error should be nil. See TestSBDBElementsAreFullPrecision.
	params.Set("full-prec", "true")

	var payload lookupPayload
	if err := p.client.GetJSON(ctx, remote.JPLSBDB, "", params, &payload); err != nil {
		return lookupPayload{}, fmt.Errorf("sbdb: %w", err)
	}

	return payload, nil
}

// queryBright issues one sb-kind-scoped bulk query against
// remote.JPLSBDBQuery for the bodies [brightConstraint] admits, sorted
// magField ascending (brightest first — confirmed live that the query API's
// `sort` parameter, distinct from ADQL-style syntax, accepts a bare field name
// with ascending as the default direction), keeps those [couldReach] passes,
// and reports how many rows SBDB sent so the caller can tell a cut answer
// from a complete one. Decodes the column-oriented response into
// resolve.Targets carrying identity, H/G (asteroids) or M1/K1 (comets), and —
// when the full set parses — osculating orbital elements (see elementFields).
//
// A nonzero at bounds each comet by where its orbit has it around that date
// rather than by its perihelion alone; see sunDistanceAround.
func (p *Provider) queryBright(ctx context.Context, sbKind, magField string, maxVMag float64, at time.Time, limit int) ([]resolve.Target, int, error) {
	fields := "full_name,spkid,H,G,class" + elementFields
	if magField == "M1" {
		fields = "full_name,spkid,M1,K1,class" + elementFields
	}

	params := url.Values{}
	params.Set("sb-kind", sbKind)
	params.Set("sb-cdata", brightConstraint(magField, maxVMag))
	params.Set("fields", fields)
	params.Set("sort", magField)
	params.Set("limit", strconv.Itoa(limit))
	// The bulk query rounds to four significant figures without it, as the
	// identify endpoint rounds to three: C/1937 C1's e comes back as 1.0002
	// for 1.000162271, and its perihelion time to a hundredth of a day. See
	// ResolveObject's own full-prec comment for what rounding costs.
	params.Set("full-prec", "true")

	var resp sbdbQueryResponse
	if err := p.client.GetJSON(ctx, remote.JPLSBDBQuery, "", params, &resp); err != nil {
		return nil, 0, fmt.Errorf("sbdb: bulk query (sb-kind=%s): %w", sbKind, err)
	}

	col := make(map[string]int, len(resp.Fields))
	for i, name := range resp.Fields {
		col[name] = i
	}

	targets := make([]resolve.Target, 0, len(resp.Data))

	for _, row := range resp.Data {
		spkID := cellString(row[col["spkid"]])

		// "class" is looked up via the two-value form (unlike the other
		// fields below, which this function always requests itself and so
		// can trust are present) since it's the one field an offline test
		// fixture might reasonably omit — col["class"] defaulting to the
		// int zero-value 0 would otherwise silently read the full_name
		// column instead of failing loudly.
		var orbitClass string
		if idx, ok := col["class"]; ok {
			orbitClass = cellString(row[idx])
		}

		var (
			eccentricity float64
			hasE         bool
		)

		if idx, ok := col["e"]; ok {
			if v, err := parseFloat(cellString(row[idx])); err == nil {
				eccentricity, hasE = v, true
			}
		}

		// Same all-or-nothing gate as ResolveObject's identify-endpoint
		// decode: HasElements is only true when every one of
		// a/i/om/w/ma/epoch parsed, never on a partial row.
		var (
			semiMajorAxis, incl, node, argp, ma    float64
			hasA, hasIncl, hasNode, hasArgp, hasMA bool
			q, tp                                  float64
			hasQ, hasTP                            bool
			epochJD                                float64
			hasEpoch                               bool
		)

		if idx, ok := col["a"]; ok {
			if v, err := parseFloat(cellString(row[idx])); err == nil {
				semiMajorAxis, hasA = v, true
			}
		}

		if idx, ok := col["i"]; ok {
			if v, err := parseFloat(cellString(row[idx])); err == nil {
				incl, hasIncl = v, true
			}
		}

		if idx, ok := col["om"]; ok {
			if v, err := parseFloat(cellString(row[idx])); err == nil {
				node, hasNode = v, true
			}
		}

		if idx, ok := col["w"]; ok {
			if v, err := parseFloat(cellString(row[idx])); err == nil {
				argp, hasArgp = v, true
			}
		}

		if idx, ok := col["ma"]; ok {
			if v, err := parseFloat(cellString(row[idx])); err == nil {
				ma, hasMA = v, true
			}
		}

		if idx, ok := col["q"]; ok {
			if v, err := parseFloat(cellString(row[idx])); err == nil {
				q, hasQ = v, true
			}
		}

		if idx, ok := col["tp"]; ok {
			if v, err := parseFloat(cellString(row[idx])); err == nil {
				tp, hasTP = v, true
			}
		}

		if idx, ok := col["epoch"]; ok {
			if v, err := parseFloat(cellString(row[idx])); err == nil {
				epochJD, hasEpoch = v, true
			}
		}

		hasAsteroidForm, hasCometForm := hasA && hasMA, hasQ && hasTP
		hasElements := hasIncl && hasNode && hasArgp && hasEpoch && (hasAsteroidForm || hasCometForm)

		t := resolve.Target{
			ID:          spkID,
			Name:        strings.TrimSpace(cellString(row[col["full_name"]])),
			SPKID:       spkID,
			Kind:        classifyKind(spkID, orbitClass, eccentricity, sbKind == "c"),
			Catalog:     "sbdb",
			HasElements: hasElements,
		}

		if hasElements {
			t.Epoch = time.FromJD(epochJD, time.TDB)
			t.Eccentricity = eccentricity
			t.Inclination = angle.Deg(incl)
			t.AscendingNode = angle.Deg(node)
			t.ArgPeriapsis = angle.Deg(argp)
			setElementForms(&t, hasAsteroidForm, semiMajorAxis, ma, hasCometForm, q, tp)
		}

		if magField == "H" {
			if v, err := parseFloat(cellString(row[col["H"]])); err == nil {
				t.H, t.HasH = v, true
			}

			if v, err := parseFloat(cellString(row[col["G"]])); err == nil {
				t.G = v
			}
		} else {
			if v, err := parseFloat(cellString(row[col["M1"]])); err == nil {
				t.M1, t.HasM1 = v, true
			}

			if v, err := parseFloat(cellString(row[col["K1"]])); err == nil {
				t.K1 = v
			}
		}

		if hasQ {
			closest := q
			if sbKind == "c" && !at.IsZero() && hasE && hasTP {
				closest = sunDistanceAround(q, eccentricity, time.FromJD(tp, time.TDB), at)
			}

			if !couldReach(t, closest, maxVMag) {
				continue
			}
		}

		targets = append(targets, t)
	}

	return targets, len(resp.Data), nil
}

// earthAphelionAU is the farthest the Earth gets from the Sun, (1 + e)·a
// with e = 0.0167. A body r from the Sun is at least r − 1.0167 AU from the
// Earth, which is what bounds how bright a body whose orbit stays outside
// the Earth's can ever look.
const earthAphelionAU = 1.0167

// couldReach reports whether a body that comes no nearer the Sun than q could
// be seen brighter than maxVMag: q is its perihelion distance, or for a dated
// search the nearest it comes around that date (sunDistanceAround).
//
// Its distance from the Sun is never less than q, and if q is beyond the
// Earth's aphelion its distance from the Earth is never less than
// q − earthAphelionAU. An asteroid's H-G magnitude is
// H + 5·log10(r·Δ) plus a phase term that is never negative when
// 0 <= G <= 1, so it is never brighter than H + 5·log10(q·(q − 1.0167)). A
// comet's total magnitude is M1 + 5·log10(Δ) + K1·log10(r), never brighter
// than M1 + 5·log10(q − 1.0167) + K1·log10(q) when K1 >= 0. A body that can
// come inside the Earth's aphelion, or whose G or K1 is outside those ranges,
// has no such bound and is kept.
func couldReach(t resolve.Target, q, maxVMag float64) bool {
	if q <= earthAphelionAU {
		return true
	}

	switch {
	case t.HasH && t.G >= 0 && t.G <= 1:
		return t.H+5*math.Log10(q*(q-earthAphelionAU)) < maxVMag
	case t.HasM1 && t.K1 >= 0:
		return t.M1+5*math.Log10(q-earthAphelionAU)+t.K1*math.Log10(q) < maxVMag
	default:
		return true
	}
}

// datedCometRows is the page a dated search asks SBDB for comets: more than
// it holds with a total magnitude, 1,995 on 2026-10-15, so that every comet
// the undated bound admits arrives and the date can rank them.
const datedCometRows = 5000

// datedWindow is how far either side of a dated search's instant a comet's
// distance from the Sun is bounded over.
//
// The orbit is propagated two-body from SBDB's osculating elements, and the
// planets move a real comet off that orbit, most of all in when it reaches a
// given distance. A month either side absorbs that much error in timing,
// which is also more than any night a caller is asking about lasts.
var datedWindow = unit.Days(30)

// sunDistanceAround returns the nearest a comet with perihelion distance q,
// eccentricity e and perihelion time tp comes to the Sun within datedWindow
// of at, in AU. A comet whose orbit cannot be propagated gets q back, which
// keeps it.
//
// Away from perihelion the distance changes monotonically, so the nearest is
// at an end of the window. A comet falling toward the Sun at the start and
// receding at the end passed perihelion inside it, and gets q.
func sunDistanceAround(q, e float64, tp, at time.Time) float64 {
	// Only the distance from the Sun is read, which no epoch or angle
	// changes: the orbit is taken as osculating at its perihelion.
	el, err := kepler.FromPerihelion(tp, tp, unit.AU(q), e, angle.Zero(), angle.Zero(), angle.Zero())
	if err != nil {
		return q
	}

	before, vBefore, errBefore := el.StateAt(at.Add(-datedWindow))
	after, vAfter, errAfter := el.StateAt(at.Add(datedWindow))

	if errBefore != nil || errAfter != nil {
		return q
	}

	if before.Dot(vBefore) < 0 && after.Dot(vAfter) > 0 {
		return q
	}

	return math.Max(q, math.Min(before.Norm(), after.Norm()))
}

// brightOffsets are the band edges [brightConstraint] splits the magnitude
// range into, relative to the bound asked for: wide where SBDB holds only a
// handful of bodies, one magnitude wide where it holds thousands.
var brightOffsets = []float64{-20, -12, -8, -6, -5, -4, -3, -2, -1, 0, 1, 2, 3, brightnessMargin}

// brightConstraint is the sb-cdata for the bodies of one kind that could be
// seen brighter than maxVMag: absolute magnitude under maxVMag +
// brightnessMargin, and, band by band, a perihelion close enough that
// [couldReach] could pass for the brightest body in the band.
//
// Without the perihelion bound the bodies with the lowest H are distant
// ones: of the 50 asteroids SBDB listed first under H 6, 9 or 11, 47 were
// trans-Neptunian objects, which are fainter than their H by some 16
// magnitudes, and Juno, Hebe and Iris were never a candidate at any bound
// (#779). SBDB cannot evaluate the bound per body, so each band uses its
// brightest edge, which admits a superset; queryBright applies the exact
// test to what comes back. Below the first edge there is no constraint, and
// for comets one more term admits any whose K1 is negative or missing, which
// the bound does not cover.
func brightConstraint(magField string, maxVMag float64) string {
	num := func(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }

	terms := []string{strconv.Quote(magField + "|LT|" + num(maxVMag+brightOffsets[0]))}

	for i := range len(brightOffsets) - 1 {
		lo, hi := maxVMag+brightOffsets[i], maxVMag+brightOffsets[i+1]

		// The farthest perihelion from which a body as bright as lo could
		// reach maxVMag, solving the bound in couldReach for q.
		x := math.Pow(10, (maxVMag-lo)/5)

		maxQ := (earthAphelionAU + math.Sqrt(earthAphelionAU*earthAphelionAU+4*x)) / 2
		if magField == "M1" {
			maxQ = earthAphelionAU + x
		}

		terms = append(terms, fmt.Sprintf(`{"AND":[%q,%q,%q]}`,
			magField+"|GE|"+num(lo), magField+"|LT|"+num(hi), "q|LT|"+num(maxQ)))
	}

	if magField == "M1" {
		top := strconv.Quote("M1|LT|" + num(maxVMag+brightnessMargin))
		terms = append(terms, `{"AND":[`+top+`,"K1|LT|0"]}`, `{"AND":[`+top+`,"K1|ND"]}`)
	}

	return `{"OR":[` + strings.Join(terms, ",") + `]}`
}

// setElementForms sets whichever of the two element forms SBDB published:
// the asteroid form (a, M at the epoch) and the comet form (q, tp, which SBDB
// labels TDB). SBDB publishes both for nearly everything; a parabola has no
// semi-major axis and so no mean anomaly, and is left with the comet form
// alone.
func setElementForms(t *resolve.Target, hasAsteroidForm bool, a, ma float64, hasCometForm bool, q, tp float64) {
	if hasAsteroidForm {
		t.SemiMajorAxis = unit.AU(a)
		t.MeanAnomaly = angle.Deg(ma)
	}

	if hasCometForm {
		t.PerihelionDistance = unit.AU(q)
		t.PerihelionTime = time.FromJD(tp, time.TDB)
	}
}

// leadingNumber matches the numeric prefix of an SBDB value, exponent
// included.
var leadingNumber = regexp.MustCompile(`^[+-]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?`)

// parseFloat extracts a float64 from a string, ignoring trailing units/notes.
//
// SBDB sometimes returns values like "3.53 (assumed)", so only the numeric
// prefix is read — and that prefix includes an exponent. SBDB writes small
// values in E-notation: C/1937 C1's mean anomaly is "-2.593805408851336E-5",
// and a prefix that stopped at the E read it as −2.59°.
func parseFloat(s string) (float64, error) {
	s = leadingNumber.FindString(strings.TrimSpace(s))

	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("sbdb: parse float: %w", err)
	}

	return v, nil
}
