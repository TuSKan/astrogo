package gaia

import (
	"bufio"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"strconv"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/internal/finite"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/votable"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

// Provider implements resolve.Provider and resolve.ConeSearcher
// explicitly pointing at Gaia DR3 to extract astrometric parameters.
type Provider struct {
	client   *remote.Client
	cache    resolve.Cache
	endpoint remote.EndpointID
}

// DefaultEndpoint is the archive a provider talks to when the caller names
// none.
//
// Gaia@AIP rather than ESA's own archive, on measured behavior rather than
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

	client := remote.Default()

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

// Resolve reports [resolve.ErrUnsupported]: Gaia is a cone-search provider and
// does not resolve names.
func (p *Provider) Resolve(_ context.Context, _ string) (resolve.Target, error) {
	return resolve.Target{}, fmt.Errorf("%w: gaia resolves positions, not names", resolve.ErrUnsupported)
}

// Search reports [resolve.ErrUnsupported], for the reason given on
// [Provider.Resolve].
func (p *Provider) Search(_ context.Context, _ string) ([]resolve.Target, error) {
	return nil, fmt.Errorf("%w: gaia resolves positions, not names", resolve.ErrUnsupported)
}

// gaiaDR3Epoch is Gaia DR3's reference epoch, J2016.0: JD 2451545.0 + 16 x
// 365.25 = 2457389.0. It was JD 2457388.5, midnight on 2016 January 1, half a
// day early (#611). Gaia's epoch is in TCB; TT is 20 s from it, which no
// proper motion notices.
var gaiaDR3Epoch = time.FromJD(2457389.0, time.TT)

// ConeSearch performs a spatial cone search via the Gaia DR3 TAP service.
//
// It returns the req.Limit sources nearest the center, nearest first, or the
// 100 nearest when req.Limit is not set. Without the ordering a TAP service
// returns the first rows it reaches, so a cone holding more sources than the
// limit would come back as an arbitrary subset of it, as likely from the edge
// as from the center.
func (p *Provider) ConeSearch(ctx context.Context, req resolve.ConeRequest) resolve.SeqIterator[resolve.Target] {
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}

	// Gaia's positions are at J2016.0, so a moving center is sought there.
	center, err := coord.PropagateEpoch(req.Center, gaiaDR3Epoch)
	if err != nil {
		return func(yield func(resolve.Target, error) bool) {
			yield(resolve.Target{}, fmt.Errorf("gaia: cone center: %w", err))
		}
	}

	rad := req.Radius.Degrees()
	if math.IsNaN(rad) || math.IsInf(rad, 0) || rad < 0 {
		return func(yield func(resolve.Target, error) bool) {
			yield(resolve.Target{}, fmt.Errorf("gaia: %w: %g deg", resolve.ErrInvalidRadius, rad))
		}
	}

	// Every number goes out exactly, as the shortest decimal that reads back
	// to the same float64. It was %f, six decimals of a degree, so a radius
	// under 1.8 mas was sent as 0, an empty cone, and cones within 3.6 mas of
	// one another shared a cache entry (#762).
	ra := strconv.FormatFloat(center.RA().Degrees(), 'f', -1, 64)
	dec := strconv.FormatFloat(center.Dec().Degrees(), 'f', -1, 64)
	radius := strconv.FormatFloat(rad, 'f', -1, 64)

	// Query gaia source for ra, dec, pmra, pmdec, parallax, the errors of
	// the position and proper motion and their correlations.
	// Ordered by an alias rather than by the DISTANCE expression itself, which
	// ADQL 2.0 does not allow in ORDER BY.
	adql := fmt.Sprintf(`SELECT TOP %d source_id, ra, dec, pmra, pmdec, parallax, ra_error, dec_error, pmra_error, pmdec_error, ra_dec_corr, ra_pmra_corr, ra_pmdec_corr, dec_pmra_corr, dec_pmdec_corr, pmra_pmdec_corr, phot_g_mean_mag, bp_rp, DISTANCE(POINT('ICRS', ra, dec), POINT('ICRS', %s, %s)) AS dist FROM gaiadr3.gaia_source WHERE 1=CONTAINS(POINT('ICRS', ra, dec), CIRCLE('ICRS', %s, %s, %s)) ORDER BY dist ASC`, limit, ra, dec, ra, dec, radius)

	// The archive is part of the key. Two providers over one cache would
	// otherwise serve each other's answers, and the whole point of holding
	// both is to be able to tell them apart.
	cacheKey := fmt.Sprintf("gaia:cone:%s:%s:%s:%s:%d", p.endpoint, ra, dec, radius, limit)
	if seq, ok := p.cache.Get(cacheKey); ok {
		return seq
	}

	v := url.Values{}
	v.Set("REQUEST", "doQuery")
	v.Set("LANG", "ADQL")
	v.Set("FORMAT", responseFormat(p.endpoint))
	v.Set("QUERY", adql)

	return func(yield func(resolve.Target, error) bool) {
		body, err := p.client.PostForm(ctx, p.endpoint, "", v)
		if err != nil {
			yield(resolve.Target{}, err)
			return
		}
		defer func() { _ = body.Close() }()

		targets, truncated, err := parseResult(body)
		if err != nil {
			yield(resolve.Target{}, err)
			return
		}

		// A cut list is not the answer to this query, so it is not kept as
		// one.
		if !truncated {
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

		if truncated {
			yield(resolve.Target{}, fmt.Errorf("gaia: %w: the archive returned %d of the %d sources asked for",
				resolve.ErrTruncated, len(targets), limit))
		}
	}
}

// responseFormat is the FORMAT this provider asks its archive for.
//
// A VOTable, because its QUERY_STATUS is the only way an archive says it cut
// an answer, and CSV has nowhere to put it. ESA's synchronous endpoint caps
// an answer at 50,000 rows and, asked for TOP 60000 as CSV, sends exactly
// 50,000 and nothing else (measured 2026-10-14; #786). ESA writes TABLEDATA,
// which the VOTable reader decodes, only for "votable_plain": "votable" and
// the IVOA media type with serialization=TABLEDATA both come back as BINARY2.
// Gaia@AIP refuses "votable_plain" and every TABLEDATA name, and answers a
// TABLEDATA VOTable to "csv", so it is left asking for that.
func responseFormat(endpoint remote.EndpointID) string {
	if endpoint == remote.GaiaTAP {
		return "votable_plain"
	}

	return "csv"
}

// parseResult decodes whichever serialization the archive actually sent.
//
// The format is read off the payload rather than off the request, the same way
// the star-map loader tells Parquet from CSV by the file's own magic. It has
// to be: ESA honors FORMAT=csv and Gaia@AIP's synchronous endpoint does not —
// it answers VOTable for FORMAT=csv, RESPONSEFORMAT=csv and text/csv alike,
// checked against the live service — so what was asked for does not determine
// what arrives. Sniffing also means a service that changes its mind is handled
// rather than discovered.
//
// A VOTable begins with an XML declaration or its root element and a CSV with
// a column name, so the first non-space byte separates them without ambiguity.
//
// truncated reports a VOTable whose QUERY_STATUS says the archive cut it. A
// CSV cannot say so, and is never reported truncated.
func parseResult(body io.Reader) (targets []resolve.Target, truncated bool, err error) {
	buf := bufio.NewReader(body)

	for {
		b, err := buf.Peek(1)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, false, nil
			}

			return nil, false, fmt.Errorf("gaia: read result: %w", err)
		}

		// Leading whitespace belongs to neither format, so it is skipped
		// rather than allowed to decide the answer.
		if b[0] == ' ' || b[0] == '\n' || b[0] == '\r' || b[0] == '\t' {
			if _, err := buf.Discard(1); err != nil {
				return nil, false, fmt.Errorf("gaia: read result: %w", err)
			}

			continue
		}

		if b[0] == '<' {
			return parseVOTable(buf)
		}

		targets, err := parseCSV(buf)

		return targets, false, err
	}
}

// parseVOTable turns a VOTable result into targets, and reports whether the
// archive said it cut them.
func parseVOTable(body io.Reader) (targets []resolve.Target, truncated bool, err error) {
	table, err := votable.Read(body)
	if err != nil {
		// A web page is the archive being down, not a malformed result, and
		// a caller has to be able to tell those apart to know whether
		// retrying is worth anything. votable spots it; remote names it.
		if errors.Is(err, votable.ErrNotVOTable) {
			return nil, false, fmt.Errorf("gaia: %w: %w", remote.ErrNotServingData, err)
		}

		return nil, false, fmt.Errorf("gaia: %w", err)
	}

	col := make(map[string]int, len(table.Fields))
	for i, f := range table.Fields {
		col[f] = i
	}

	targets = make([]resolve.Target, 0, len(table.Rows))

	for _, row := range table.Rows {
		if t, ok := targetFromRow(row, col); ok {
			targets = append(targets, t)
		}
	}

	return targets, table.Truncated, nil
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
// Shared by both serializations deliberately. They differ only in how bytes
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

		// NaN and the infinities parse, and VOTable allows NaN for a null
		// float. Read as absent, as an empty cell is (#1080).
		return finite.Parse(s)
	}

	id, ok := cell("source_id")
	if !ok || id == "" {
		return resolve.Target{}, false
	}

	raDeg, okRA := number("ra")
	decDeg, okDec := number("dec")

	if !okRA || !okDec || !finite.Position(raDeg, decDeg) {
		// A row with an unparseable, missing or impossible position is
		// useless for astrometric cross-matching and worse than absent — drop
		// it rather than reporting a fake (0,0) as if it were real.
		return resolve.Target{}, false
	}

	// The designation is Gaia's own, as its designation column spells it,
	// and the identity key every other catalog's cross-identification of
	// the source carries: SIMBAD lists Barnard's star as "Gaia DR3
	// 4472832130942575872". A bare source ID is in no naming scheme.
	t := resolve.Target{
		ID:          id,
		Name:        "Gaia DR3 " + id,
		Designation: "Gaia DR3 " + id,
		Kind:        resolve.KindStar,
		Coord:       coord.NewICRS(angle.Deg(raDeg), angle.Deg(decDeg)),
		HasCoord:    true,
		Epoch:       gaiaDR3Epoch,
		Catalog:     "Gaia DR3",
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

	t.AstrometricError = astrometricError(number)

	// Johnson V from Gaia G and the BP−RP color.
	gMag, okG := number("phot_g_mean_mag")
	if !okG {
		return t, true
	}

	if bpRp, okC := number("bp_rp"); okC {
		// Outside the colors Riello et al. (2021) fitted the relation over,
		// the cubic extrapolates — several magnitudes adrift by BP−RP = 7 —
		// and an L dwarf or a carbon star is reported with no V rather than
		// with an invented one. This used to inline its own copy of the
		// cubic and apply it at any color (#530).
		v, ok := magnitude.GaiaGToJohnsonV(gMag, bpRp)
		if !ok {
			return t, true
		}

		t.VMag = v
		t.HasVMag = true

		return t, true
	}

	// No color — G stands in for V, within about 0.3 mag for most stars.
	t.VMag = gMag
	t.HasVMag = true

	return t, true
}

// astrometricError reads a row's position and proper-motion errors, in mas
// and mas/yr, and their correlations, through number. A cell the row does
// not have is left zero, which AstrometricError reads as unpublished.
func astrometricError(number func(string) (float64, bool)) resolve.AstrometricError {
	mas := func(name string) angle.Angle {
		v, _ := number(name)

		return angle.Arcsec(v / 1000)
	}

	corr := func(name string) float64 {
		v, _ := number(name)

		return v
	}

	return resolve.AstrometricError{
		RA: mas("ra_error"), Dec: mas("dec_error"),
		PmRA: mas("pmra_error"), PmDec: mas("pmdec_error"),
		RADec:     corr("ra_dec_corr"),
		RAPmRA:    corr("ra_pmra_corr"),
		RAPmDec:   corr("ra_pmdec_corr"),
		DecPmRA:   corr("dec_pmra_corr"),
		DecPmDec:  corr("dec_pmdec_corr"),
		PmRAPmDec: corr("pmra_pmdec_corr"),
	}
}
