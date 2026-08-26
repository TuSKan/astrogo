package gaia

import (
	"bufio"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/votable"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/remote/api"
	"github.com/TuSKan/astrogo/time"
)

// Provider implements resolve.Provider and resolve.ConeSearcher
// explicitly pointing at Gaia DR3 to extract astrometric parameters.
type Provider struct {
	client   *api.Client
	cache    resolve.Cache
	endpoint remote.EndpointID
}

// DefaultEndpoint is the archive a provider talks to when the caller names
// none.
//
// Gaia@AIP rather than ESA's own archive, on measured behaviour rather than
// preference. Both serve the same DR3 tables - the data release is fixed, so
// there is no question of one being more current - and on the same schema
// query AIP answers in about three seconds against ESA's ten. ESA has also
// been unreachable for a whole working day while AIP answered throughout, and
// this package's own network test carries a helper whose comment records that
// ESA "routinely accepts the connection and then never answers".
//
// A default is a decision made on behalf of callers who have not thought about
// it, so it should be the endpoint most likely to answer. ESA stays registered
// and selectable, and is what the cross-archive validation compares against;
// see TestArchivesAgree.
const DefaultEndpoint = remote.GaiaAIP

// New creates a Gaia DR3 catalog provider against one archive.
//
// The zero endpoint selects [DefaultEndpoint]. Naming one explicitly is how a
// caller reaches a specific archive - ESA's, for instance, when the point is
// to compare the two rather than to get an answer.
func New(endpoint remote.EndpointID) (*Provider, error) {
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}

	client, err := api.NewClient(endpoint)
	if err != nil {
		return nil, fmt.Errorf("gaia: %w", err)
	}

	return &Provider{
		client:   client,
		cache:    resolve.NewMapCache(),
		endpoint: endpoint,
	}, nil
}

// Endpoint reports which archive this provider queries.
func (p *Provider) Endpoint() remote.EndpointID { return p.endpoint }

// Name returns the provider identifier.
func (p *Provider) Name() string { return "gaia" }

// Capabilities returns the set of supported resolution operations.
func (p *Provider) Capabilities() []resolve.Capability {
	return []resolve.Capability{resolve.CapConeSearch}
}

// Resolve always returns false for Gaia (no name resolution).
func (p *Provider) Resolve(_ context.Context, _ string) (resolve.Target, bool) {
	return resolve.Target{}, false
}

// Search always returns nil for Gaia (no name search).
func (p *Provider) Search(_ context.Context, _ string) []resolve.Target {
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

	// The archive is part of the key. Two providers over one cache would
	// otherwise serve each other's answers, and the whole point of holding
	// both is to be able to tell them apart.
	cacheKey := fmt.Sprintf("gaia:cone:%s:%f:%f:%f:%d", p.endpoint, ra, dec, rad, limit)
	if seq, ok := p.cache.Get(cacheKey); ok {
		return seq
	}

	v := url.Values{}
	v.Set("REQUEST", "doQuery")
	v.Set("LANG", "ADQL")
	v.Set("FORMAT", "csv")
	v.Set("QUERY", adql)

	return func(yield func(resolve.Target, error) bool) {
		body, err := p.client.PostForm(ctx, p.endpoint, "", v)
		if err != nil {
			yield(resolve.Target{}, err)
			return
		}
		defer func() { _ = body.Close() }()

		targets, err := parseResult(body)
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

// parseResult decodes whichever serialisation the archive actually sent.
//
// The format is read off the payload rather than off the request, the same way
// the star-map loader tells Parquet from CSV by the file's own magic. It has
// to be: ESA honours FORMAT=csv and Gaia@AIP's synchronous endpoint does not —
// it answers VOTable for FORMAT=csv, RESPONSEFORMAT=csv and text/csv alike,
// checked against the live service — so what was asked for does not determine
// what arrives. Sniffing also means a service that changes its mind is handled
// rather than discovered.
//
// A VOTable begins with an XML declaration or its root element and a CSV with
// a column name, so the first non-space byte separates them without ambiguity.
func parseResult(body io.Reader) ([]resolve.Target, error) {
	buf := bufio.NewReader(body)

	for {
		b, err := buf.Peek(1)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, nil
			}

			return nil, fmt.Errorf("gaia: read result: %w", err)
		}

		// Leading whitespace belongs to neither format, so it is skipped
		// rather than allowed to decide the answer.
		if b[0] == ' ' || b[0] == '\n' || b[0] == '\r' || b[0] == '\t' {
			if _, err := buf.Discard(1); err != nil {
				return nil, fmt.Errorf("gaia: read result: %w", err)
			}

			continue
		}

		if b[0] == '<' {
			return parseVOTable(buf)
		}

		return parseCSV(buf)
	}
}

// parseVOTable turns a VOTable result into targets.
func parseVOTable(body io.Reader) ([]resolve.Target, error) {
	table, err := votable.Read(body)
	if err != nil {
		return nil, fmt.Errorf("gaia: %w", err)
	}

	col := make(map[string]int, len(table.Fields))
	for i, f := range table.Fields {
		col[f] = i
	}

	targets := make([]resolve.Target, 0, len(table.Rows))

	for _, row := range table.Rows {
		if t, ok := targetFromRow(row, col); ok {
			targets = append(targets, t)
		}
	}

	return targets, nil
}

// parseCSV turns a CSV result into targets.
func parseCSV(body io.Reader) ([]resolve.Target, error) {
	reader := csv.NewReader(body)

	header, err := reader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
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
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("gaia: read CSV: %w", err)
		}

		if t, ok := targetFromRow(row, col); ok {
			targets = append(targets, t)
		}
	}

	return targets, nil
}

// targetFromRow turns one result row into a target, reporting false for a row
// that cannot be used.
//
// Shared by both serialisations deliberately. They differ only in how bytes
// become cells; what a cell means — which column is the identifier, how a
// proper motion is scaled, how V follows from G and BP−RP — is the same
// question with the same answer, and two copies of it would be two things to
// keep in step.
//
// Every column is looked up before it is indexed. A missing name yields index
// zero from a map, so the previous form read column zero as though it were the
// one asked for, and a short row indexed past its end; both are reachable from
// a service that returns fewer columns than the query named.
func targetFromRow(row []string, col map[string]int) (resolve.Target, bool) {
	cell := func(name string) (string, bool) {
		i, ok := col[name]
		if !ok || i >= len(row) {
			return "", false
		}

		return row[i], true
	}

	number := func(name string) (float64, bool) {
		s, ok := cell(name)
		if !ok || s == "" {
			return 0, false
		}

		v, err := strconv.ParseFloat(s, 64)

		return v, err == nil
	}

	id, ok := cell("source_id")
	if !ok || id == "" {
		return resolve.Target{}, false
	}

	raDeg, okRA := number("ra")
	decDeg, okDec := number("dec")

	if !okRA || !okDec {
		// A row with an unparseable or missing position is useless for
		// astrometric cross-matching and worse than absent — drop it rather
		// than reporting a fake (0,0) as if it were real.
		return resolve.Target{}, false
	}

	t := resolve.Target{
		ID:       id,
		Name:     "Gaia DR3 " + id,
		Kind:     resolve.KindStar,
		Coord:    coord.NewICRS(angle.Deg(raDeg), angle.Deg(decDeg)),
		HasCoord: true,
		Epoch:    time.FromJD(2457388.5, time.UTC), // Gaia DR3 epoch is J2016.0
		Catalog:  "Gaia DR3",
	}

	if v, ok := number("pmra"); ok {
		t.PmRA = angle.Arcsec(v / 1000.0)
	}

	if v, ok := number("pmdec"); ok {
		t.PmDec = angle.Arcsec(v / 1000.0)
	}

	if v, ok := number("parallax"); ok {
		t.Parallax = angle.Arcsec(v / 1000.0)
	}

	// Johnson V from Gaia G and the BP−RP colour.
	gMag, okG := number("phot_g_mean_mag")
	if !okG {
		return t, true
	}

	if bpRp, okC := number("bp_rp"); okC {
		// Gaia DR3 documentation Table 5.9 is tabulated as G minus the target
		// band, so this is G − V and V is G less it. See
		// [magnitude.GaiaGToJohnsonV] for the evidence on the direction.
		gMinusV := -0.02704 + 0.01424*bpRp - 0.2156*bpRp*bpRp + 0.01426*bpRp*bpRp*bpRp
		t.VMag = gMag - gMinusV
		t.HasVMag = true

		return t, true
	}

	// No colour — G stands in for V, within about 0.3 mag for most stars.
	t.VMag = gMag
	t.HasVMag = true

	return t, true
}
