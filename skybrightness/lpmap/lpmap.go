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
	// naturalLuminanceCdM2 is the natural zenith background, 0.171168465 mcd/m²
	// ≡ 22.00 V mag/arcsec² (lightpollutionmap.info/help.html).
	naturalLuminanceCdM2 = 1.71168465e-4
	// magLuminanceZeroPoint is the SQM zero-point, 1.08e8 mcd/m².
	magLuminanceZeroPoint = 1.08e5
)

// Sentinel errors.
var (
	// ErrNoAPIKey is returned when no API key is configured.
	ErrNoAPIKey = errors.New("lpmap: no API key (use WithAPIKey or set LIGHTPOLLUTIONMAP_KEY)")
	// ErrBadResponse is returned when the API response cannot be parsed.
	ErrBadResponse = errors.New("lpmap: unexpected API response")
)

// Client queries the lightpollutionmap.info QueryRaster service.
type Client struct {
	apiKey string
	layer  string
	client *remote.Client
}

// Option configures a Client.
type Option func(*Client)

// WithAPIKey sets the QueryRaster API key, overriding LIGHTPOLLUTIONMAP_KEY.
func WithAPIKey(key string) Option { return func(c *Client) { c.apiKey = key } }

// WithLayer overrides the raster layer (default "wa_2015").
func WithLayer(layer string) Option { return func(c *Client) { c.layer = layer } }

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
		apiKey: os.Getenv(apiKeyEnv),
		layer:  defaultLayer,
		client: client,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// SQM returns the TOTAL zenith sky surface brightness (V mag/arcsec²) at the
// given geodetic latitude and longitude (degrees), combining the World Atlas
// artificial brightness with the natural background. This is a self-contained
// answer to "how bright is the sky here" — do not feed it into a
// skybrightness.CompositeModel alongside Airglow/Zodiacal/Moonlight, since
// those already add their own natural-background components and would
// double-count it. Use Floor for the composable, artificial-only value.
func (c *Client) SQM(ctx context.Context, latDeg, lonDeg float64) (skybrightness.SurfaceBrightnessV, error) {
	art, err := c.artificialBrightness(ctx, latDeg, lonDeg)
	if err != nil {
		return 0, err
	}

	return artificialToSQM(art), nil
}

// Floor returns a skybrightness.Floor built from the site's resolved
// ARTIFICIAL-ONLY sky brightness (World Atlas 2015 layer) — consistent with
// the artificial-only contract skybrightness/atlas's Falchi/VIIRS providers
// use (see skybrightness/atlas/doc.go), so it composes safely with
// Airglow/Zodiacal/Moonlight in a skybrightness.CompositeModel without
// double-counting the natural background. Use SQM instead for a
// self-contained total (artificial+natural) brightness value.
func (c *Client) Floor(ctx context.Context, latDeg, lonDeg float64) (skybrightness.Floor, error) {
	art, err := c.artificialBrightness(ctx, latDeg, lonDeg)
	if err != nil {
		return skybrightness.Floor{}, err
	}

	return skybrightness.NewFloorSQM(skybrightness.SurfaceBrightnessFromMcdM2(art)), nil
}

// artificialBrightness fetches the World Atlas artificial sky brightness
// (mcd/m²) at the site.
func (c *Client) artificialBrightness(ctx context.Context, latDeg, lonDeg float64) (float64, error) {
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
