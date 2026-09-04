package sbdb

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/remote/api"
	"github.com/TuSKan/astrogo/time"
)

// ErrAPIError indicates a SBDB API error response.
var ErrAPIError = errors.New("sbdb: API error")

// Provider implements resolve.Provider and resolve.ObjectResolver for SBDB.
type Provider struct {
	client *api.Client
	cache  resolve.Cache
}

// New creates a new SBDB catalog provider.
func New() *Provider {
	client, err := api.NewClient(remote.JPLSBDB)
	if err != nil {
		panic(err) // unregistered endpoint would be a programmer error
	}

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

// Resolve performs exact-match resolution for a query.
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

	params := url.Values{}
	// Switch to using Lookup API explicitly targeted via sstr
	params.Set("sstr", req.Query)
	// Request physical parameters to get H, G, M1, k1 for magnitude computation.
	params.Set("phys-par", "true")
	// Without this SBDB rounds every orbital element to three significant
	// figures — Eros comes back with a = 1.46 and e = 0.223, not 1.458243716
	// and 0.2228779628. That is not a display detail: propagating the rounded
	// elements two-body puts Eros 690,000 km from its own kernel *at the epoch
	// of osculation*, where osculating elements are exact by construction and
	// the error should be nil. See TestSBDBElementsAreFullPrecision.
	params.Set("full-prec", "true")

	return func(yield func(resolve.Target, error) bool) {
		var payload struct {
			Object *struct {
				SpkID      string `json:"spkid"`
				FullName   string `json:"fullname"`
				Des        string `json:"des"`
				Kind       string `json:"kind"`
				OrbitClass struct {
					Code string `json:"code"`
				} `json:"orbit_class"`
			} `json:"object"`
			Message string `json:"message"`
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

		if err := p.client.GetJSON(ctx, remote.JPLSBDB, "", params, &payload); err != nil {
			yield(resolve.Target{}, err)
			return
		}

		if payload.Message != "" {
			// This means either multiple matches or error
			// The JSON payload includes generic text if multiple
			// We skip multiple matching to keep it exact resolution for lookup API
			yield(resolve.Target{}, fmt.Errorf("%w: %s", ErrAPIError, payload.Message))
			return
		}

		if payload.Object == nil {
			yield(resolve.Target{}, nil) // empty
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
			}
		}

		epochJD, epochErr := parseFloat(payload.Orbit.Epoch)
		hasElements := hasA && hasIncl && hasNode && hasArgp && hasMA && epochErr == nil

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
			t.SemiMajorAxis = semiMajorAxis
			t.Eccentricity = eccentricity
			t.Inclination = angle.Deg(incl)
			t.AscendingNode = angle.Deg(node)
			t.ArgPeriapsis = angle.Deg(argp)
			t.MeanAnomaly = angle.Deg(ma)
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
			case "diameter":
				if v, err := parseFloat(pp.Value); err == nil {
					t.Diameter = v
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

// brightnessMargin is a cushion added to a requested magnitude bound
// before using it as an absolute-magnitude (H/M1) prefilter — a body needs
// some brightening from a favorable-opposition geometry to go from its
// intrinsic absolute magnitude to what's actually seen, so the real filter
// (computed apparent magnitude, done downstream once real per-body
// geometry is known — see plan.VisibleTonight) needs candidates a tight
// H/M1 bound alone would incorrectly exclude here.
//
// 4.0 is calibrated against the largest real-world opposition brightening
// among the well-known bright asteroids — Ceres, the biggest correction of
// the three (H=3.4, best-ever apparent ≈6.7, a ≈3.3 mag correction) — with
// headroom. It deliberately does NOT chase extreme close-approach NEA
// brightening (e.g. 99942 Apophis's 2029 flyby: H=19.7 but a predicted
// peak apparent magnitude ≈3.1, a ≈16.6 mag correction): a margin that
// large would defeat the purpose of a prefilter (empirically, H<20 for
// asteroids alone numbers in the hundreds of thousands) and belongs to a
// fundamentally different query — a dedicated close-approach lookup
// against JPL's Close Approach Data API — which this library doesn't
// implement. This is a deliberate, documented scope limitation, not an
// oversight.
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

// SearchBright returns every asteroid and comet SBDB tracks with an
// absolute magnitude (H for asteroids, M1 for comets) within
// brightnessMargin of req.MaxVMag, brightest (lowest H/M1) first. This is
// Stage 1 of a two-stage design — it returns identity plus H/G or M1/K1
// only, never a real position or computed apparent magnitude, since that
// needs per-body ephemeris state the catalog layer deliberately doesn't
// depend on (Stage 2 lives in plan.VisibleTonight, which already imports
// both catalog and ephemeris).
//
// req.Limit defaults to 50 per kind (asteroids and comets are queried and
// capped separately, so up to 100 candidates total). Because queryBright
// sorts server-side by H/M1 ascending before this cap is applied, the
// candidates kept are always the genuinely brightest ones within
// brightnessMargin, not an arbitrary subset — a real bug in an earlier
// version of this function (no sort, so a raw 500-per-kind cap could
// arbitrarily drop bright candidates while keeping fainter ones nearer the
// H/M1 boundary) that live testing against the real SBDB Query API caught.
func (p *Provider) SearchBright(ctx context.Context, req resolve.BrightRequest) resolve.SeqIterator[resolve.Target] {
	cacheKey := fmt.Sprintf("bright:%f:%d", req.MaxVMag, req.Limit)
	if seq, ok := p.cache.Get(cacheKey); ok {
		return seq
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}

	maxAbsMag := req.MaxVMag + brightnessMargin

	return func(yield func(resolve.Target, error) bool) {
		asteroids, err := p.queryBright(ctx, "a", "H", maxAbsMag, limit)
		if err != nil {
			yield(resolve.Target{}, err)
			return
		}

		comets, err := p.queryBright(ctx, "c", "M1", maxAbsMag, limit)
		if err != nil {
			yield(resolve.Target{}, err)
			return
		}

		targets := make([]resolve.Target, 0, len(asteroids)+len(comets))
		targets = append(targets, asteroids...)
		targets = append(targets, comets...)

		if err := p.cache.Set(cacheKey, targets); err != nil {
			yield(resolve.Target{}, err)
			return
		}

		for _, t := range targets {
			if !yield(t, nil) {
				return
			}
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
// classifyKind, now decoded once alongside the rest.
const elementFields = ",e,a,i,om,w,ma,epoch"

// queryBright issues one sb-kind-scoped bulk query against
// remote.JPLSBDBQuery, filtering by magField < maxVal, sorted magField
// ascending (brightest first — confirmed live that the query API's `sort`
// parameter, distinct from ADQL-style syntax, accepts a bare field name
// with ascending as the default direction) so that limit always keeps the
// genuinely brightest candidates rather than an arbitrary subset. Decodes
// the column-oriented response into resolve.Targets carrying identity,
// H/G (asteroids) or M1/K1 (comets), and — when the full set parses —
// osculating orbital elements (see elementFields).
func (p *Provider) queryBright(ctx context.Context, sbKind, magField string, maxVal float64, limit int) ([]resolve.Target, error) {
	fields := "full_name,spkid,H,G,class" + elementFields
	if magField == "M1" {
		fields = "full_name,spkid,M1,K1,class" + elementFields
	}

	params := url.Values{}
	params.Set("sb-kind", sbKind)
	params.Set("sb-cdata", fmt.Sprintf(`{"AND":["%s|LT|%g"]}`, magField, maxVal))
	params.Set("fields", fields)
	params.Set("sort", magField)
	params.Set("limit", strconv.Itoa(limit))

	var resp sbdbQueryResponse
	if err := p.client.GetJSON(ctx, remote.JPLSBDBQuery, "", params, &resp); err != nil {
		return nil, fmt.Errorf("sbdb: bulk query (sb-kind=%s): %w", sbKind, err)
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

		var eccentricity float64

		if idx, ok := col["e"]; ok {
			if v, err := parseFloat(cellString(row[idx])); err == nil {
				eccentricity = v
			}
		}

		// Same all-or-nothing gate as ResolveObject's identify-endpoint
		// decode: HasElements is only true when every one of
		// a/i/om/w/ma/epoch parsed, never on a partial row.
		var (
			semiMajorAxis, incl, node, argp, ma    float64
			hasA, hasIncl, hasNode, hasArgp, hasMA bool
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

		if idx, ok := col["epoch"]; ok {
			if v, err := parseFloat(cellString(row[idx])); err == nil {
				epochJD, hasEpoch = v, true
			}
		}

		hasElements := hasA && hasIncl && hasNode && hasArgp && hasMA && hasEpoch

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
			t.SemiMajorAxis = semiMajorAxis
			t.Eccentricity = eccentricity
			t.Inclination = angle.Deg(incl)
			t.AscendingNode = angle.Deg(node)
			t.ArgPeriapsis = angle.Deg(argp)
			t.MeanAnomaly = angle.Deg(ma)
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

		targets = append(targets, t)
	}

	return targets, nil
}

// parseFloat extracts a float64 from a string, ignoring trailing units/notes.
func parseFloat(s string) (float64, error) {
	s = strings.TrimSpace(s)
	// SBDB sometimes returns values like "3.53" or "3.53 (assumed)"
	// Take only the numeric prefix.
	for i, c := range s {
		if c != '-' && c != '+' && c != '.' && (c < '0' || c > '9') {
			s = s[:i]
			break
		}
	}

	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, fmt.Errorf("sbdb: parse float: %w", err)
	}

	return v, nil
}
