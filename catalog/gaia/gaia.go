package gaia

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/coord"
)

const tapSyncURL = "https://gea.esac.esa.int/tap-server/tap/sync"

// Provider implements catalog.Provider and catalog.ConeSearcher 
// explicitly pointing at Gaia DR3 to extract astrometric parameters.
type Provider struct {
	client *catalog.Client
	cache  catalog.Cache
}

func New() *Provider {
	return &Provider{
		client: catalog.NewClient(),
		cache:  catalog.NewArrowCache(),
	}
}

func (p *Provider) Name() string { return "gaia" }

func (p *Provider) Capabilities() []catalog.Capability {
	return []catalog.Capability{catalog.CapConeSearch}
}

func (p *Provider) Resolve(query string) (catalog.Target, bool) {
	return catalog.Target{}, false
}

func (p *Provider) Search(query string) []catalog.Target {
	return nil 
}

func (p *Provider) ConeSearch(ctx context.Context, req catalog.ConeRequest) catalog.SeqIterator[catalog.Target] {
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}

	ra := req.Center.RA().Degrees()
	dec := req.Center.Dec().Degrees()
	rad := req.Radius.Degrees()

	// Query gaia source for ra, dec, pmra, pmdec, parallax
	adql := fmt.Sprintf(`SELECT TOP %d 
	source_id, ra, dec, pmra, pmdec, parallax
	FROM gaiadr3.gaia_source
	WHERE 1=CONTAINS(POINT('ICRS', ra, dec), CIRCLE('ICRS', %f, %f, %f))`, limit, ra, dec, rad)

	cacheKey := fmt.Sprintf("gaia:cone:%f:%f:%f:%d", ra, dec, rad, limit)
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
		return catalog.SliceSeq([]catalog.Target{})
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return func(yield func(catalog.Target, error) bool) {
		resp, err := p.client.Do(httpReq)
		if err != nil {
			yield(catalog.Target{}, err)
			return
		}
		defer resp.Body.Close()

		targets, err := parseCSV(resp.Body)
		if err != nil {
			yield(catalog.Target{}, err)
			return
		}

		p.cache.Set(cacheKey, targets)

		for _, t := range targets {
			if !yield(t, nil) {
				return
			}
		}
	}
}

func parseCSV(body io.Reader) ([]catalog.Target, error) {
	reader := csv.NewReader(body)
	header, err := reader.Read()
	if err != nil {
		if err == io.EOF {
			return nil, nil
		}
		return nil, fmt.Errorf("gaia: failed to read CSV header: %w", err)
	}

	col := make(map[string]int)
	for i, h := range header {
		col[h] = i
	}

	var targets []catalog.Target

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		id := row[col["source_id"]]
		raDeg, _ := strconv.ParseFloat(row[col["ra"]], 64)
		decDeg, _ := strconv.ParseFloat(row[col["dec"]], 64)
		// PMs and parallax omitted from target struct mapping currently, standard ICRS is used
		
		t := catalog.Target{
			ID:          id,
			Name:        "Gaia DR3 " + id,
			Kind:        catalog.KindStar,
			Coord:       coord.NewICRS(angle.Deg(raDeg), angle.Deg(decDeg)),
			Catalog:     "Gaia DR3",
		}
		targets = append(targets, t)
	}
	return targets, nil
}
