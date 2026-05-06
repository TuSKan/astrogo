package vizier

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

const tapSyncURL = "http://tapvizier.u-strasbg.fr/TAPVizieR/tap/sync"

// Provider implements resolve.Provider and resolve.ConeSearcher
// for querying tables hosted on VizieR via TAP ADQL.
type Provider struct {
	client *resolve.Client
	cache  resolve.Cache
}

func New() *Provider {
	return &Provider{
		client: resolve.NewClient(),
		cache:  resolve.NewMapCache(),
	}
}

func (p *Provider) Name() string { return "vizier" }

func (p *Provider) Capabilities() []resolve.Capability {
	return []resolve.Capability{resolve.CapConeSearch}
}

func (p *Provider) Resolve(query string) (resolve.Target, bool) {
	return resolve.Target{}, false // Not supported directly, use ConeSearch
}

func (p *Provider) Search(query string) []resolve.Target {
	return nil
}

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
	raj2000 as ra, dej2000 as dec
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
		return resolve.SliceSeq([]resolve.Target{})
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return func(yield func(resolve.Target, error) bool) {
		resp, err := p.client.Do(httpReq)
		if err != nil {
			yield(resolve.Target{}, err)
			return
		}
		defer resp.Body.Close()

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

func parseCSV(body io.Reader) ([]resolve.Target, error) {
	reader := csv.NewReader(body)
	if _, err := reader.Read(); err != nil { // discard header for now and assume exact select order
		return nil, err
	}
	// TODO: implement standard schema extraction or hardcode based on SELECT order.
	return []resolve.Target{}, nil // Scaffolded
}
