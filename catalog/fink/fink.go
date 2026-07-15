package fink

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/remote"

	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"
)

// defaultVersion is the SSOFT release to query. The API defaults to the
// current month, which may not exist yet; pin to a known-good release.
var defaultVersion = "2025.04"

// Sentinel errors for FINK/SSOFT operations.
var (
	ErrNoIdentifier    = errors.New("fink: need number or name")
	ErrHTTPStatus      = errors.New("fink: unexpected HTTP status")
	ErrResponseFormat  = errors.New("fink: unexpected response format")
	ErrRemoteException = errors.New("fink: remote exception in response")
	ErrSSOFTError      = errors.New("fink: SSOFT error response")
	ErrInvalidParquet  = errors.New("fink: not a valid parquet file")
)

// ssoRecord stores the fitted sHG1G2 parameters extracted from SSOFT.
type ssoRecord struct {
	Name   string
	Number int64

	// Per-band absolute magnitudes. Suffix _1 = ZTF g, _2 = ZTF r.
	H1, H2       float64
	ErrH1, ErrH2 float64

	// Phase function parameters per band.
	G1_1, G1_2 float64
	G2_1, G2_2 float64

	// Spin-geometry (sHG1G2 extension).
	R      float64 // Oblateness ∈ (0,1], 1 = sphere
	Alpha0 float64 // Spin axis RA (deg, J2000)
	Delta0 float64 // Spin axis Dec (deg, J2000)

	// Shape ratios.
	AB, AC float64 // a/b and a/c ellipsoid ratios

	// Fit quality.
	Fit    int     // 0 = success
	Status int     // ≥1 = converged
	NObs   int     // Number of observations
	RMS    float64 // Fit RMS (mag)
}

// Provider implements resolve.Provider for the FINK SSOFT table.
//
// It supports two modes:
//   - Per-object JSON queries via /api/v1/ssoft?sso_number=N (fast, no cache needed)
//   - Full-table parquet download for bulk indexing (lazy, cached in memory)
type Provider struct {
	loadErr  error
	client   *remote.Client
	byNumber map[int64]*ssoRecord
	byName   map[string]*ssoRecord
	version  string
	mu       sync.RWMutex
	loaded   bool
}

// New returns a Provider with the default SSOFT version.
func New() *Provider {
	return NewWithVersion(defaultVersion)
}

// NewWithVersion returns a Provider targeting a specific SSOFT release (e.g. "2025.04").
func NewWithVersion(version string) *Provider {
	client, err := remote.NewClientFor(remote.FINK)
	if err != nil {
		panic(err) // unregistered endpoint would be a programmer error
	}

	return &Provider{
		client:  client,
		version: version,
	}
}

// Name returns the provider identifier.
func (p *Provider) Name() string { return "fink" }

// Capabilities returns the set of supported resolution operations.
func (p *Provider) Capabilities() []resolve.Capability {
	return []resolve.Capability{resolve.CapObjectResolution, resolve.CapFullCatalog}
}

// Resolve returns the first matching Target for the given query.
// For numeric queries, uses the fast single-object JSON API.
func (p *Provider) Resolve(query string) (resolve.Target, bool) {
	q := strings.TrimSpace(query)

	// Fast path: numeric lookup via single-object JSON endpoint.
	if n, err := strconv.ParseInt(q, 10, 64); err == nil {
		rec, err := p.querySingle(context.Background(), n, "")
		if err == nil && rec != nil {
			return p.recordToTarget(rec), true
		}
	}

	// Try name-based lookup via single-object JSON.
	rec, err := p.querySingle(context.Background(), 0, q)
	if err == nil && rec != nil {
		return p.recordToTarget(rec), true
	}

	// Fall back to bulk table lookup.
	if err := p.ensureLoaded(); err != nil {
		return resolve.Target{}, false
	}

	if rec := p.lookupCached(q); rec != nil {
		return p.recordToTarget(rec), true
	}

	return resolve.Target{}, false
}

// Search resolves a query (IAU number or name) against the SSOFT table.
func (p *Provider) Search(query string) []resolve.Target {
	tgt, ok := p.Resolve(query)
	if !ok {
		return nil
	}

	return []resolve.Target{tgt}
}

// ResolveObject implements resolve.ObjectResolver.
func (p *Provider) ResolveObject(_ context.Context, req resolve.ObjectRequest) resolve.SeqIterator[resolve.Target] {
	tgt, ok := p.Resolve(req.Query) //nolint:contextcheck // Resolve is interface-bound; bulk download path uses Background
	if !ok {
		return resolve.SliceSeq([]resolve.Target{})
	}

	return resolve.SliceSeq([]resolve.Target{tgt})
}

// Loaded reports whether the full SSOFT table has been downloaded and indexed.
func (p *Provider) Loaded() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.loaded
}

// Count returns the number of indexed SSO records (0 if bulk table not loaded).
func (p *Provider) Count() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return len(p.byNumber)
}

// ── Single-object JSON API ───────────────────────────────────────────────────

// querySingle queries the SSOFT endpoint for a single object via JSON.
// Pass number > 0 for numeric lookup, or name != "" for name lookup.
func (p *Provider) querySingle(ctx context.Context, number int64, name string) (_ *ssoRecord, err error) {
	payload := map[string]any{
		"output-format": "json",
		"version":       p.version,
		"flavor":        "SHG1G2",
	}

	switch {
	case number > 0:
		payload["sso_number"] = number
	case name != "":
		payload["sso_name"] = name
	default:
		return nil, ErrNoIdentifier
	}

	body, err := p.client.PostJSON(ctx, remote.FINK, "", payload)
	if err != nil {
		var httpErr *remote.HTTPError
		if errors.As(err, &httpErr) {
			return nil, fmt.Errorf("%w: %d: %s", ErrHTTPStatus, httpErr.StatusCode, httpErr.Body[:min(200, len(httpErr.Body))])
		}

		return nil, fmt.Errorf("fink: ssoft request: %w", err)
	}
	defer func() {
		cerr := body.Close()
		if cerr != nil {
			err = errors.Join(err, cerr)
		}
	}()

	data, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("fink: reading response: %w", err)
	}

	// Single-object returns a JSON object; full table returns an array.
	// Try object first.
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		// Try array of objects.
		var arr []map[string]any

		err2 := json.Unmarshal(data, &arr)
		if err2 != nil || len(arr) == 0 {
			return nil, ErrResponseFormat
		}

		obj = arr[0]
	}

	// Check for error responses.
	if _, hasErr := obj["RemoteException"]; hasErr {
		return nil, ErrRemoteException
	}

	rec := &ssoRecord{
		Name:   jsonStr(obj, "sso_name"),
		Number: jsonInt(obj, "sso_number"),
		H1:     jsonF64(obj, "H_1"),
		H2:     jsonF64(obj, "H_2"),
		ErrH1:  jsonF64(obj, "err_H_1"),
		ErrH2:  jsonF64(obj, "err_H_2"),
		G1_1:   jsonF64(obj, "G1_1"),
		G1_2:   jsonF64(obj, "G1_2"),
		G2_1:   jsonF64(obj, "G2_1"),
		G2_2:   jsonF64(obj, "G2_2"),
		R:      jsonF64(obj, "R"),
		Alpha0: jsonF64(obj, "alpha0"),
		Delta0: jsonF64(obj, "delta0"),
		AB:     jsonF64(obj, "a_b"),
		AC:     jsonF64(obj, "a_c"),
		Fit:    int(jsonF64(obj, "fit")),
		Status: int(jsonF64(obj, "status")),
		NObs:   int(jsonF64(obj, "n_obs")),
		RMS:    jsonF64(obj, "rms"),
	}

	return rec, nil
}

// JSON helpers for parsing SSOFT response fields.
func jsonF64(m map[string]any, key string) float64 {
	v, ok := m[key]
	if !ok || v == nil {
		return math.NaN()
	}

	switch val := v.(type) {
	case float64:
		return val
	case json.Number:
		f, _ := val.Float64()
		return f
	case string:
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return math.NaN()
		}

		return f
	}

	return math.NaN()
}

func jsonInt(m map[string]any, key string) int64 {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}

	switch val := v.(type) {
	case float64:
		return int64(val)
	case json.Number:
		i, _ := val.Int64()
		return i
	case string:
		i, _ := strconv.ParseInt(val, 10, 64)
		return i
	}

	return 0
}

func jsonStr(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}

	if s, ok := v.(string); ok {
		return s
	}

	return fmt.Sprintf("%v", v)
}

// ── Target conversion ────────────────────────────────────────────────────────

// recordToTarget converts an ssoRecord to a resolve.Target.
// Uses r-band (filter 2) for H, G1, G2 as it is closer to Johnson V.
func (p *Provider) recordToTarget(rec *ssoRecord) resolve.Target {
	t := resolve.Target{
		ID:          strconv.FormatInt(rec.Number, 10),
		Name:        rec.Name,
		Designation: strconv.FormatInt(rec.Number, 10),
		Kind:        "Asteroid",
		Catalog:     "fink",
	}

	// Use r-band (filter 2) as primary — closer to V than g-band.
	if !math.IsNaN(rec.H2) {
		t.H = rec.H2
		t.HasH = true
	} else if !math.IsNaN(rec.H1) {
		t.H = rec.H1
		t.HasH = true
	}

	// G1, G2 phase parameters.
	if !math.IsNaN(rec.G1_2) && !math.IsNaN(rec.G2_2) {
		t.G1 = rec.G1_2
		t.G2 = rec.G2_2
		t.HasG1G2 = true
	} else if !math.IsNaN(rec.G1_1) && !math.IsNaN(rec.G2_1) {
		t.G1 = rec.G1_1
		t.G2 = rec.G2_1
		t.HasG1G2 = true
	}

	// Spin axis.
	if !math.IsNaN(rec.Alpha0) && !math.IsNaN(rec.Delta0) {
		t.SpinRA = rec.Alpha0
		t.SpinDec = rec.Delta0
		t.HasSpin = true
	}

	// Oblateness.
	if !math.IsNaN(rec.R) && rec.R > 0 && rec.R <= 1 {
		t.Oblateness = rec.R
		t.HasOblateness = true
	}

	return t
}

// ── Bulk table (parquet) ─────────────────────────────────────────────────────

// lookupCached resolves a query string against the cached bulk index.
func (p *Provider) lookupCached(query string) *ssoRecord {
	p.mu.RLock()
	defer p.mu.RUnlock()

	q := strings.TrimSpace(query)
	if n, err := strconv.ParseInt(q, 10, 64); err == nil {
		if rec, ok := p.byNumber[n]; ok {
			return rec
		}
	}

	key := strings.ToLower(q)
	if rec, ok := p.byName[key]; ok {
		return rec
	}

	return nil
}

// ensureLoaded downloads and indexes the SSOFT parquet table on first call.
func (p *Provider) ensureLoaded() error {
	p.mu.RLock()

	if p.loaded {
		p.mu.RUnlock()
		return nil
	}

	if p.loadErr != nil {
		err := p.loadErr
		p.mu.RUnlock()

		return err
	}

	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.loaded {
		return nil
	}

	records, err := p.downloadSSOFT()
	if err != nil {
		p.loadErr = fmt.Errorf("fink: failed to load SSOFT: %w", err)
		return p.loadErr
	}

	p.byNumber = make(map[int64]*ssoRecord, len(records))
	p.byName = make(map[string]*ssoRecord, len(records))

	for i := range records {
		rec := &records[i]
		if rec.Fit != 0 || rec.Status < 1 {
			continue
		}

		p.byNumber[rec.Number] = rec
		if rec.Name != "" {
			p.byName[strings.ToLower(rec.Name)] = rec
		}
	}

	p.loaded = true

	return nil
}

// downloadSSOFT fetches the full SSOFT parquet table from the FINK API.
func (p *Provider) downloadSSOFT() (_ []ssoRecord, err error) {
	payload := map[string]any{
		"output-format": "parquet",
		"version":       p.version,
		"flavor":        "SHG1G2",
	}

	body, err := p.client.PostJSON(context.Background(), remote.FINK, "", payload)
	if err != nil {
		var httpErr *remote.HTTPError
		if errors.As(err, &httpErr) {
			return nil, fmt.Errorf("%w: %d: %s", ErrHTTPStatus, httpErr.StatusCode, httpErr.Body[:min(200, len(httpErr.Body))])
		}

		return nil, fmt.Errorf("SSOFT request: %w", err)
	}
	defer func() {
		cerr := body.Close()
		if cerr != nil {
			err = errors.Join(err, cerr)
		}
	}()

	data, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("reading SSOFT response: %w", err)
	}

	// Check for JSON error responses (the API returns 200 even on errors).
	if len(data) < 1000 && len(data) > 0 && data[0] == '{' {
		return nil, fmt.Errorf("%w: %s", ErrSSOFTError, string(data[:min(200, len(data))]))
	}

	// Validate parquet magic bytes (PAR1).
	if len(data) < 4 || data[0] != 0x50 || data[1] != 0x41 || data[2] != 0x52 || data[3] != 0x31 {
		return nil, fmt.Errorf("%w: size=%d", ErrInvalidParquet, len(data))
	}

	// Write to temp file (parquet reader requires seekable input).
	tmp, err := os.CreateTemp("", "astrogo-ssoft-*.parquet")
	if err != nil {
		return nil, fmt.Errorf("creating temp file: %w", err)
	}

	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(data); err != nil {
		return nil, fmt.Errorf("writing temp parquet: %w", err)
	}

	return p.readParquet(tmpName)
}

// readParquet extracts ssoRecords from a SSOFT parquet file.
func (p *Provider) readParquet(path string) (_ []ssoRecord, err error) {
	rdr, err := file.OpenParquetFile(path, false)
	if err != nil {
		return nil, fmt.Errorf("opening parquet: %w", err)
	}

	defer func() {
		cerr := rdr.Close()
		if cerr != nil {
			err = errors.Join(err, cerr)
		}
	}()

	arrowRdr, err := pqarrow.NewFileReader(rdr, pqarrow.ArrowReadProperties{}, nil)
	if err != nil {
		return nil, fmt.Errorf("creating arrow reader: %w", err)
	}

	tbl, err := arrowRdr.ReadTable(context.Background())
	if err != nil {
		return nil, fmt.Errorf("reading table: %w", err)
	}
	defer tbl.Release()

	nRows := int(tbl.NumRows())
	schema, _ := arrowRdr.Schema()

	// Build column name → index map.
	colIdx := make(map[string]int, schema.NumFields())
	for i, f := range schema.Fields() {
		colIdx[f.Name] = i
	}

	// Helper to read float64 column values.
	getF64 := func(colName string, row int) float64 {
		idx, ok := colIdx[colName]
		if !ok {
			return math.NaN()
		}

		col := tbl.Column(idx)
		for _, chunk := range col.Data().Chunks() {
			if row < chunk.Len() {
				if chunk.IsNull(row) {
					return math.NaN()
				}

				v := chunk.GetOneForMarshal(row)
				switch fv := v.(type) {
				case float64:
					return fv
				case float32:
					return float64(fv)
				case json.Number:
					f, _ := fv.Float64()
					return f
				}

				return math.NaN()
			}

			row -= chunk.Len()
		}

		return math.NaN()
	}

	getInt := func(colName string, row int) int64 {
		idx, ok := colIdx[colName]
		if !ok {
			return 0
		}

		col := tbl.Column(idx)
		for _, chunk := range col.Data().Chunks() {
			if row < chunk.Len() {
				if chunk.IsNull(row) {
					return 0
				}

				v := chunk.GetOneForMarshal(row)
				switch iv := v.(type) {
				case int32:
					return int64(iv)
				case int64:
					return iv
				case float64:
					return int64(iv)
				}

				return 0
			}

			row -= chunk.Len()
		}

		return 0
	}

	getStr := func(colName string, row int) string {
		idx, ok := colIdx[colName]
		if !ok {
			return ""
		}

		col := tbl.Column(idx)
		for _, chunk := range col.Data().Chunks() {
			if row < chunk.Len() {
				if chunk.IsNull(row) {
					return ""
				}

				v := chunk.GetOneForMarshal(row)
				if s, ok := v.(string); ok {
					return s
				}

				return fmt.Sprintf("%v", v)
			}

			row -= chunk.Len()
		}

		return ""
	}

	records := make([]ssoRecord, nRows)
	for i := range nRows {
		records[i] = ssoRecord{
			Name:   getStr("sso_name", i),
			Number: getInt("sso_number", i),
			H1:     getF64("H_1", i),
			H2:     getF64("H_2", i),
			ErrH1:  getF64("err_H_1", i),
			ErrH2:  getF64("err_H_2", i),
			G1_1:   getF64("G1_1", i),
			G1_2:   getF64("G1_2", i),
			G2_1:   getF64("G2_1", i),
			G2_2:   getF64("G2_2", i),
			R:      getF64("R", i),
			Alpha0: getF64("alpha0", i),
			Delta0: getF64("delta0", i),
			AB:     getF64("a_b", i),
			AC:     getF64("a_c", i),
			Fit:    int(getInt("fit", i)),
			Status: int(getInt("status", i)),
			NObs:   int(getInt("n_obs", i)),
			RMS:    getF64("rms", i),
		}
	}

	return records, nil
}
