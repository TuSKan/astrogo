package vizier

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
)

const tapSyncURL = "http://tapvizier.u-strasbg.fr/TAPVizieR/tap/sync"

// ErrUnexpectedSchema indicates the CSV response is missing a column the
// parser depends on, e.g. because VizieR's TAP schema for the queried table
// changed underneath ConeSearch's hardcoded ADQL SELECT clause.
var ErrUnexpectedSchema = errors.New("vizier: unexpected response schema")

// Provider implements resolve.Provider and resolve.ConeSearcher
// for querying tables hosted on VizieR via TAP ADQL.
type Provider struct {
	client *resolve.Client
	cache  resolve.Cache
}

// New creates a new VizieR catalog provider.
func New() *Provider {
	return &Provider{
		client: resolve.NewClient(),
		cache:  resolve.NewMapCache(),
	}
}

// Name returns the provider identifier.
func (p *Provider) Name() string { return "vizier" }

// Capabilities returns the set of supported resolution operations.
func (p *Provider) Capabilities() []resolve.Capability {
	return []resolve.Capability{resolve.CapConeSearch}
}

// Resolve always returns false for VizieR (use ConeSearch instead).
func (p *Provider) Resolve(_ string) (resolve.Target, bool) {
	return resolve.Target{}, false // Not supported directly, use ConeSearch
}

// Search always returns nil for VizieR (use ConeSearch instead).
func (p *Provider) Search(_ string) []resolve.Target {
	return nil
}

// ConeSearch performs a spatial cone search via the VizieR service.
func (p *Provider) ConeSearch(ctx context.Context, req resolve.ConeRequest) resolve.SeqIterator[resolve.Target] {
	// VizieR requires specifying a catalog index/table if we do ADQL.
	// Since ConeRequest doesn't specify 'table', we might need to rely purely on VizieR's standard catalogs
	// OR require the user to encode it somehow.
	// Wait, the easiest way to do a vizier cone search across standard catalogues is using their REST/ConeSearch API, not TAP.
	// But let's assume they want the standard II/246 (2MASS) for generic lookups if none specified.
	// We will create a flexible TAP generator.
	table := "II/246/out" // 2MASS point source catalog as a generic fallback baseline

	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}

	// CIRCLE receives coordinates in DEGREES for ADQL natively.
	ra := req.Center.RA().Degrees()
	dec := req.Center.Dec().Degrees()
	rad := req.Radius.Degrees()

	adql := fmt.Sprintf(`SELECT TOP %d
	"2MASS" as designation, raj2000 as ra, dej2000 as dec
	FROM "%s"
	WHERE 1=CONTAINS(POINT('ICRS', raj2000, dej2000), CIRCLE('ICRS', %f, %f, %f))`, limit, table, ra, dec, rad)

	cacheKey := fmt.Sprintf("vizier:cone:%f:%f:%f:%d", ra, dec, rad, limit)
	if seq, ok := p.cache.Get(cacheKey); ok {
		return seq
	}

	v := url.Values{}
	v.Set("REQUEST", "doQuery")
	v.Set("LANG", "ADQL")
	v.Set("FORMAT", "csv")
	v.Set("QUERY", adql)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, tapSyncURL, strings.NewReader(v.Encode()))
	if err != nil {
		return func(yield func(resolve.Target, error) bool) {
			yield(resolve.Target{}, fmt.Errorf("vizier: new request: %w", err))
		}
	}

	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return func(yield func(resolve.Target, error) bool) {
		resp, err := p.client.Do(httpReq)
		if err != nil {
			yield(resolve.Target{}, err)
			return
		}
		defer func() {
			cerr := resp.Body.Close()
			if cerr != nil {
				yield(resolve.Target{}, cerr)
			}
		}()

		targets, err := parseCSV(resp.Body)
		if err != nil {
			yield(resolve.Target{}, err)
			return
		}

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

// parseCSV extracts designation/ra/dec rows from the ADQL query's CSV
// response. Columns are located by header name rather than assumed position,
// so the parser stays correct if the SELECT clause in ConeSearch is reordered.
func parseCSV(body io.Reader) ([]resolve.Target, error) {
	reader := csv.NewReader(body)

	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("vizier: read header: %w", err)
	}

	col := make(map[string]int, len(header))
	for i, name := range header {
		col[strings.ToLower(strings.TrimSpace(name))] = i
	}

	desigIdx, raIdx, decIdx := col["designation"], col["ra"], col["dec"]
	if _, ok := col["designation"]; !ok {
		return nil, fmt.Errorf("%w: missing %q column", ErrUnexpectedSchema, "designation")
	}

	if _, ok := col["ra"]; !ok {
		return nil, fmt.Errorf("%w: missing %q column", ErrUnexpectedSchema, "ra")
	}

	if _, ok := col["dec"]; !ok {
		return nil, fmt.Errorf("%w: missing %q column", ErrUnexpectedSchema, "dec")
	}

	var targets []resolve.Target

	for {
		record, err := reader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return nil, fmt.Errorf("vizier: read row: %w", err)
		}

		designation := strings.TrimSpace(record[desigIdx])

		raDeg, err := strconv.ParseFloat(strings.TrimSpace(record[raIdx]), 64)
		if err != nil {
			return nil, fmt.Errorf("vizier: parse ra %q: %w", record[raIdx], err)
		}

		decDeg, err := strconv.ParseFloat(strings.TrimSpace(record[decIdx]), 64)
		if err != nil {
			return nil, fmt.Errorf("vizier: parse dec %q: %w", record[decIdx], err)
		}

		targets = append(targets, resolve.Target{
			Catalog:     "vizier",
			Name:        designation,
			Designation: designation,
			Kind:        resolve.KindStar,
			Coord:       coord.NewICRS(angle.Deg(raDeg), angle.Deg(decDeg)),
		})
	}

	return targets, nil
}
