package lpmap

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness"
)

// defaultLayer is the World Atlas 2015 (Falchi et al. 2016) artificial-brightness layer.
const defaultLayer = "wa_2015"

// apiKeyEnv is the environment variable consulted for the API key by default.
const apiKeyEnv = "LIGHTPOLLUTIONMAP_KEY"

// Photometric constants for the brightness → magnitude conversion:
//
//	L[cd/m²] = magLuminanceZeroPoint · 10^(−0.4·m)
//
// anchored to the natural zenith background naturalLuminanceCdM2 ≡ 22.0 V
// mag/arcsec² (Falchi et al. 2016).
const (
	// naturalLuminanceCdM2 is skybrightness.NaturalZenithMcdM2 (the single
	// source for this constant, mcd/m²) converted to cd/m² for this file's
	// own unit convention.
	naturalLuminanceCdM2 = skybrightness.NaturalZenithMcdM2 * 1e-3
	// magLuminanceZeroPoint is the SQM zero-point, 1.08e8 mcd/m².
	magLuminanceZeroPoint = 1.08e5
)

// layerUnit is the physical unit a QueryRaster "ql" layer reports its
// values in — the two documented families are not interchangeable (see
// WithLayer's doc comment).
type layerUnit int

const (
	// unitMcdM2 is World Atlas artificial brightness, mcd/m².
	unitMcdM2 layerUnit = iota
	// unitRadiance is raw VIIRS-DNB upward radiance, nW·cm⁻²·sr⁻¹.
	unitRadiance
)

// viirsLayerPrefix marks the raw-radiance layer family; the rest of the
// name is the composite's year.
const viirsLayerPrefix = "viirs_"

// VIIRSLayer names the raw-radiance annual composite for year, for use
// with [WithLayer]:
//
//	lpmap.New(lpmap.WithLayer(lpmap.VIIRSLayer(2025)))
//
// Exists because the default layer is "wa_2015" — World Atlas 2015, the
// higher-FIDELITY but decade-old source. A caller who wants this client on
// the freshest data has to say so: nothing here probes upstream for which
// years exist (that is atlas.NewestVIIRSYear's job, and it costs a network
// round trip). Naming a year upstream does not carry surfaces as an error
// from [Client.SQM]/[Client.Floor], never as a silently wrong value.
func VIIRSLayer(year int) string { return fmt.Sprintf("%s%d", viirsLayerPrefix, year) }

// layerUnitFor reports the physical unit a QueryRaster "ql" layer reports
// its values in, and whether the layer name is one this client knows how
// to interpret at all.
//
// This deliberately matches the "viirs_<year>" FAMILY by shape rather than
// enumerating specific years: every VIIRS composite reports radiance
// whatever its year, so a hardcoded year list would add no safety while
// silently rejecting each new year upstream publishes (they run 2012
// through at least 2025 and grow annually). Existence is the server's
// call — it answers with an error for a year it does not carry. What the
// client must get right, and all this decides, is the UNIT: reading a
// radiance value as mcd/m² produces a plausible-looking wrong brightness
// rather than an obvious failure, which is exactly the bug this replaced.
func layerUnitFor(layer string) (layerUnit, bool) {
	if layer == defaultLayer {
		return unitMcdM2, true
	}

	year, found := strings.CutPrefix(layer, viirsLayerPrefix)
	if !found || len(year) != 4 {
		return 0, false
	}

	if _, err := strconv.Atoi(year); err != nil {
		return 0, false
	}

	return unitRadiance, true
}

// Sentinel errors.
var (
	// ErrNoAPIKey is returned when no API key is configured.
	ErrNoAPIKey = errors.New("lpmap: no API key (use WithAPIKey or set LIGHTPOLLUTIONMAP_KEY)")
	// ErrBadResponse is returned when the API response cannot be parsed.
	ErrBadResponse = errors.New("lpmap: unexpected API response")
	// ErrUnknownLayer is returned when the configured layer belongs to
	// neither known "ql" family (see layerUnitFor), so its unit — and
	// therefore how to interpret its numeric value — isn't known.
	ErrUnknownLayer = errors.New("lpmap: unknown raster layer")
)

// Client queries the lightpollutionmap.info QueryRaster service.
type Client struct {
	apiKey            string
	layer             string
	radianceSlope     float64
	radianceZeroPoint float64
	client            *remote.Client
}

// Option configures a Client.
type Option func(*Client)

// WithAPIKey sets the QueryRaster API key, overriding LIGHTPOLLUTIONMAP_KEY.
func WithAPIKey(key string) Option { return func(c *Client) { c.apiKey = key } }

// WithLayer overrides the raster layer (default "wa_2015"). QueryRaster
// layers span two incompatible units: "wa_2015" (World Atlas, mcd/m²
// artificial brightness) and "viirs_<year>" (raw VIIRS-DNB radiance,
// nW·cm⁻²·sr⁻¹) — Client dispatches on the family to interpret its
// value correctly; a name in neither family returns ErrUnknownLayer from
// SQM/Floor rather than being silently misread. See WithRadianceCoefficients
// to override the radiance→brightness fit used for a "viirs_*" layer.
func WithLayer(layer string) Option { return func(c *Client) { c.layer = layer } }

// WithRadianceCoefficients overrides the log-linear radiance→brightness fit
// (SB = slope·log₁₀(radiance) + zeroPoint) used for a "viirs_*" layer —
// e.g. with a VIIRS-DNB-calibrated pair once one is published; see
// skybrightness.RadianceToArtificialSB for the default coefficients'
// provenance. Meaningless for the "wa_2015" layer.
func WithRadianceCoefficients(slope, zeroPoint float64) Option {
	return func(c *Client) { c.radianceSlope, c.radianceZeroPoint = slope, zeroPoint }
}

// WithHTTPClient sets a custom HTTP client (transport, proxy, TLS config).
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.client.HTTPClient = h } }

// New creates a Client. The API key defaults to the LIGHTPOLLUTIONMAP_KEY
// environment variable unless overridden with WithAPIKey. Requests go
// through remote.Client (retry/backoff on transient failures and 429/5xx
// responses — the daily request quota, see doc.go, is a usage-pattern
// limit, not a burst rate, so there is nothing to throttle beyond that).
func New(opts ...Option) *Client {
	client, err := remote.NewClientFor(remote.LightPollution)
	if err != nil {
		panic(err) // unregistered endpoint would be a programmer error
	}

	c := &Client{
		apiKey:            os.Getenv(apiKeyEnv),
		layer:             defaultLayer,
		radianceSlope:     skybrightness.DefaultRadianceSlope,
		radianceZeroPoint: skybrightness.DefaultRadianceZeroPoint,
		client:            client,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// SQM returns the TOTAL zenith sky surface brightness (V mag/arcsec²) at the
// given geodetic latitude and longitude (degrees), combining the layer's
// artificial brightness with the natural background. This is a self-contained
// answer to "how bright is the sky here" — do not feed it into a
// skybrightness.CompositeModel alongside Airglow/Zodiacal/Moonlight, since
// those already add their own natural-background components and would
// double-count it. Use Floor for the composable, artificial-only value.
func (c *Client) SQM(ctx context.Context, latDeg, lonDeg float64) (skybrightness.SurfaceBrightnessV, error) {
	total, _, err := c.resolveBrightness(ctx, latDeg, lonDeg)
	return total, err
}

// Floor returns a skybrightness.Floor built from the site's resolved
// ARTIFICIAL-ONLY sky brightness — consistent with the artificial-only
// contract skybrightness/atlas's Falchi/VIIRS providers use (see
// skybrightness/atlas/doc.go), so it composes safely with
// Airglow/Zodiacal/Moonlight in a skybrightness.CompositeModel without
// double-counting the natural background. Use SQM instead for a
// self-contained total (artificial+natural) brightness value.
func (c *Client) Floor(ctx context.Context, latDeg, lonDeg float64) (skybrightness.Floor, error) {
	_, artificial, err := c.resolveBrightness(ctx, latDeg, lonDeg)
	if err != nil {
		return skybrightness.Floor{}, err
	}

	return skybrightness.NewFloorSQM(artificial), nil
}

// resolveBrightness fetches the raw QueryRaster value for the client's
// configured layer and converts it to both a TOTAL and an ARTIFICIAL-ONLY
// V-band zenith surface brightness, dispatching on the layer's physical
// unit (see WithLayer's doc comment): mixing a "viirs_*" layer's raw
// radiance into the mcd/m² path used to silently produce a
// plausible-looking wrong number.
func (c *Client) resolveBrightness(ctx context.Context, latDeg, lonDeg float64) (total, artificial skybrightness.SurfaceBrightnessV, err error) {
	unit, ok := layerUnitFor(c.layer)
	if !ok {
		return 0, 0, fmt.Errorf("%w: %q", ErrUnknownLayer, c.layer)
	}

	raw, err := c.queryRaster(ctx, latDeg, lonDeg)
	if err != nil {
		return 0, 0, err
	}

	if unit == unitRadiance {
		if raw <= 0 {
			inf := skybrightness.SurfaceBrightnessV(math.Inf(1))
			return inf, inf, nil
		}

		total = skybrightness.SurfaceBrightnessV(c.radianceSlope*math.Log10(raw) + c.radianceZeroPoint)
		artificial = skybrightness.RadianceToArtificialSB(raw, c.radianceSlope, c.radianceZeroPoint)

		return total, artificial, nil
	}

	// unitMcdM2: negative values are clamped to zero (no light) rather
	// than passed through, which would otherwise send a negative argument
	// into SurfaceBrightnessFromMcdM2's log10.
	mcdM2 := max(raw, 0)

	return artificialToSQM(mcdM2), skybrightness.SurfaceBrightnessFromMcdM2(mcdM2), nil
}

// queryRaster fetches the raw QueryRaster numeric value (unit depends on
// the configured layer — see WithLayer) at the site.
func (c *Client) queryRaster(ctx context.Context, latDeg, lonDeg float64) (float64, error) {
	if c.apiKey == "" {
		return 0, ErrNoAPIKey
	}

	q := url.Values{}
	q.Set("ql", c.layer)
	q.Set("qt", "point")
	q.Set("qd", fmt.Sprintf("%.6f,%.6f", lonDeg, latDeg)) // API order is lon,lat
	q.Set("key", c.apiKey)

	resp, err := c.client.Get(ctx, remote.LightPollution, "", q)
	if err != nil {
		var httpErr *remote.HTTPError
		if errors.As(err, &httpErr) {
			return 0, fmt.Errorf("%w: status %d: %s", ErrBadResponse, httpErr.StatusCode, strings.TrimSpace(httpErr.Body))
		}

		return 0, fmt.Errorf("lpmap: http: %w", err)
	}
	defer func() { _ = resp.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp, 1<<16))
	if err != nil {
		return 0, fmt.Errorf("lpmap: read body: %w", err)
	}

	return parseBrightness(string(body))
}

// parseBrightness extracts the brightness value from the CSV point-query
// response, taking the last numeric token (point responses are short CSV).
func parseBrightness(body string) (float64, error) {
	fields := strings.FieldsFunc(body, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t' || r == ';'
	})

	for _, f := range slices.Backward(fields) {
		if v, err := strconv.ParseFloat(strings.TrimSpace(f), 64); err == nil {
			return v, nil
		}
	}

	return 0, fmt.Errorf("%w: no numeric value in %q", ErrBadResponse, strings.TrimSpace(body))
}

// artificialToSQM converts a World Atlas artificial brightness (mcd/m²) to a
// total zenith V-band surface brightness (mag/arcsec²) by adding the natural
// background in linear luminance.
func artificialToSQM(artificialMcdM2 float64) skybrightness.SurfaceBrightnessV {
	if artificialMcdM2 < 0 {
		artificialMcdM2 = 0
	}

	lTot := naturalLuminanceCdM2 + artificialMcdM2*1e-3 // mcd/m² → cd/m²

	return skybrightness.SurfaceBrightnessV(-2.5 * math.Log10(lTot/magLuminanceZeroPoint))
}
