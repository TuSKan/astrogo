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
// changed underneath ConeSearch's ADQL SELECT clause.
var ErrUnexpectedSchema = errors.New("vizier: unexpected response schema")

// ErrUnknownTable indicates a ConeRequest named a VizieR table not present
// in this package's schema registry (see tables.go). Rather than guess at
// that table's RA/Dec/designation column names, ConeSearch requires the
// table to be registered first.
var ErrUnknownTable = errors.New("vizier: unknown table")

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
//
// req.Table selects which VizieR table to query (e.g. "I/239/hip_main");
// the empty string defaults to the 2MASS point-source catalog
// (II/246/out), this package's original behavior. Querying a table not
// present in this package's schema registry (tables.go) returns
// ErrUnknownTable.
func (p *Provider) ConeSearch(ctx context.Context, req resolve.ConeRequest) resolve.SeqIterator[resolve.Target] {
	tableName := req.Table
	if tableName == "" {
		tableName = defaultTable
	}

	schema, ok := tableSchemas[tableName]
	if !ok {
		return func(yield func(resolve.Target, error) bool) {
			yield(resolve.Target{}, fmt.Errorf("%w: %q", ErrUnknownTable, tableName))
		}
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}

	// CIRCLE receives coordinates in DEGREES for ADQL natively.
	ra := req.Center.RA().Degrees()
	dec := req.Center.Dec().Degrees()
	rad := req.Radius.Degrees()

	adql := fmt.Sprintf(`SELECT TOP %d
	%s as designation, %s as ra, %s as dec
	FROM "%s"
	WHERE 1=CONTAINS(POINT('ICRS', %s, %s), CIRCLE('ICRS', %f, %f, %f))`,
		limit, schema.DesigCol, schema.RACol, schema.DecCol, tableName, schema.RACol, schema.DecCol, ra, dec, rad)

	// The table name is part of the cache key: two different tables queried
	// with the same cone would otherwise collide on the same entry.
	cacheKey := fmt.Sprintf("vizier:cone:%s:%f:%f:%f:%d", tableName, ra, dec, rad, limit)
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

		targets, err := parseCSV(resp.Body, schema.Kind)
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
// response, tagging every row with kind. Columns are located by header name
// rather than assumed position, so the parser stays correct if the SELECT
// clause in ConeSearch is reordered.
func parseCSV(body io.Reader, kind resolve.Kind) ([]resolve.Target, error) {
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
			Kind:        kind,
			Coord:       coord.NewICRS(angle.Deg(raDeg), angle.Deg(decDeg)),
			HasCoord:    true,
		})
	}

	return targets, nil
}
