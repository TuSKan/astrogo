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
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

const tapSyncURL = "https://gea.esac.esa.int/tap-server/tap/sync"

// Provider implements resolve.Provider and resolve.ConeSearcher
// explicitly pointing at Gaia DR3 to extract astrometric parameters.
type Provider struct {
	client *resolve.Client
	cache  resolve.Cache
}

// New creates a new Gaia DR3 catalog provider.
func New() *Provider {
	return &Provider{
		client: resolve.NewClient(),
		cache:  resolve.NewMapCache(),
	}
}

// Name returns the provider identifier.
func (p *Provider) Name() string { return "gaia" }

// Capabilities returns the set of supported resolution operations.
func (p *Provider) Capabilities() []resolve.Capability {
	return []resolve.Capability{resolve.CapConeSearch}
}

// Resolve always returns false for Gaia (no name resolution).
func (p *Provider) Resolve(_ string) (resolve.Target, bool) {
	return resolve.Target{}, false
}

// Search always returns nil for Gaia (no name search).
func (p *Provider) Search(_ string) []resolve.Target {
	return nil
}

// ConeSearch performs a spatial cone search via the Gaia DR3 TAP service.
func (p *Provider) ConeSearch(ctx context.Context, req resolve.ConeRequest) resolve.SeqIterator[resolve.Target] {
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}

	ra := req.Center.RA().Degrees()
	dec := req.Center.Dec().Degrees()
	rad := req.Radius.Degrees()

	// Query gaia source for ra, dec, pmra, pmdec, parallax
	adql := fmt.Sprintf(`SELECT TOP %d source_id, ra, dec, pmra, pmdec, parallax, phot_g_mean_mag, bp_rp FROM gaiadr3.gaia_source WHERE 1=CONTAINS(POINT('ICRS', ra, dec), CIRCLE('ICRS', %f, %f, %f))`, limit, ra, dec, rad)

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
		return resolve.SliceSeq([]resolve.Target{})
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

		err = p.cache.Set(cacheKey, targets)
		if err != nil {
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

	var targets []resolve.Target

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("gaia: read CSV: %w", err)
		}

		id := row[col["source_id"]]
		raDeg, _ := strconv.ParseFloat(row[col["ra"]], 64)
		decDeg, _ := strconv.ParseFloat(row[col["dec"]], 64)

		t := resolve.Target{
			ID:       id,
			Name:     "Gaia DR3 " + id,
			Kind:     resolve.KindStar,
			Coord:    coord.NewICRS(angle.Deg(raDeg), angle.Deg(decDeg)),
			HasCoord: true,
			Epoch:    time.FromJD(2457388.5, time.UTC), // Gaia DR3 epoch is J2016.0
			Catalog:  "Gaia DR3",
		}

		if pmRAStr, ok := col["pmra"]; ok && row[pmRAStr] != "" {
			if v, err := strconv.ParseFloat(row[pmRAStr], 64); err == nil {
				t.PmRA = angle.Arcsec(v / 1000.0)
			}
		}

		if pmDecStr, ok := col["pmdec"]; ok && row[pmDecStr] != "" {
			if v, err := strconv.ParseFloat(row[pmDecStr], 64); err == nil {
				t.PmDec = angle.Arcsec(v / 1000.0)
			}
		}

		if plxStr, ok := col["parallax"]; ok && row[plxStr] != "" {
			if v, err := strconv.ParseFloat(row[plxStr], 64); err == nil {
				t.Parallax = angle.Arcsec(v / 1000.0)
			}
		}
		// Compute Johnson V from Gaia G + BP-RP colour.
		if gIdx, ok := col["phot_g_mean_mag"]; ok && row[gIdx] != "" {
			if gMag, err := strconv.ParseFloat(row[gIdx], 64); err == nil {
				if bpRpIdx, ok2 := col["bp_rp"]; ok2 && row[bpRpIdx] != "" {
					if bpRp, err2 := strconv.ParseFloat(row[bpRpIdx], 64); err2 == nil {
						// Gaia DR3 polynomial: V − G = −0.02704 + 0.01424·c − 0.2156·c² + 0.01426·c³
						dV := -0.02704 + 0.01424*bpRp - 0.2156*bpRp*bpRp + 0.01426*bpRp*bpRp*bpRp
						t.VMag = gMag + dV
						t.HasVMag = true
					}
				} else {
					// No colour — use G as approximate V (±0.3 mag for most stars).
					t.VMag = gMag
					t.HasVMag = true
				}
			}
		}

		targets = append(targets, t)
	}

	return targets, nil
}
