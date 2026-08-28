// Package passband resolves photometric passbands from the Spanish Virtual
// Observatory's Filter Profile Service.
//
// A [github.com/TuSKan/astrogo/magnitude.Passband] is a transmission curve
// plus two things the curve does not carry: the detector convention it is
// meant to be integrated under, and the zero point of the magnitude system.
// Both are properties of how the filter was calibrated rather than of its
// shape, and both change the answer — integrating an energy-calibrated curve
// as if it were photon-counting tilts the result across the band, and a wrong
// zero point moves every magnitude by a constant. SVO publishes all three
// together, which is the reason to fetch a passband rather than transcribe one.
//
// # Which filters
//
// SVO identifies a filter by a string like "Generic/Bessell.V". Bessell (1990)
// is the standard realisation of the Johnson-Cousins UBVRI system, so
// "Generic/Bessell.U" through "Generic/Bessell.I" are the five bands
// Masana et al. (2024) Table 1 tabulates. The service carries several thousand
// others; nothing here is specific to Bessell, and the identifier is passed
// through unchanged.
//
// # Caching
//
// A filter profile is a published calibration and does not change, so a
// fetched curve is written to the endpoint's cache directory and reused. The
// cache is keyed by the filter identifier.
package passband

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/remote/api"
	"github.com/TuSKan/astrogo/remote/file"
	"github.com/TuSKan/astrogo/unit"
)

// ErrService reports that the service answered with something unusable.
var ErrService = errors.New("passband: filter profile service")

// angstromPerNM converts the service's wavelength unit to this module's.
const angstromPerNM = 10

// Fetch returns the passband SVO publishes under id, for example
// "Generic/Bessell.V".
//
// The curve is cached on first use and read from the cache afterwards, so
// repeated calls cost one request in total. A cache that cannot be opened is
// not an error: the fetch proceeds and the result simply is not saved.
func Fetch(ctx context.Context, id string) (magnitude.Passband, error) {
	if strings.TrimSpace(id) == "" {
		return magnitude.Passband{}, fmt.Errorf("%w: no filter identifier", ErrService)
	}

	bucket, key, cacheErr := cacheLocation(ctx, id)
	if cacheErr == nil {
		if r, err := bucket.NewReader(ctx, key, nil); err == nil {
			defer func() { _ = r.Close() }()

			// A cached profile that will not parse is not worth failing over:
			// fall through and fetch it again.
			if band, err := Parse(r); err == nil {
				return band, nil
			}
		}
	}

	client, err := api.NewClient(remote.SVOFilterProfile)
	if err != nil {
		return magnitude.Passband{}, fmt.Errorf("%w: %w", ErrService, err)
	}

	body, err := client.Get(ctx, remote.SVOFilterProfile, "", url.Values{"ID": {id}})
	if err != nil {
		return magnitude.Passband{}, fmt.Errorf("%w: %w", ErrService, err)
	}

	defer func() { _ = body.Close() }()

	raw, err := io.ReadAll(body)
	if err != nil {
		return magnitude.Passband{}, fmt.Errorf("%w: %w", ErrService, err)
	}

	band, err := Parse(strings.NewReader(string(raw)))
	if err != nil {
		return magnitude.Passband{}, err
	}

	if cacheErr == nil {
		// A cache write that fails costs a request next time and nothing else.
		_ = file.Save(ctx, bucket, key, strings.NewReader(string(raw)))
	}

	return band, nil
}

// cacheLocation resolves where a filter's profile is kept.
func cacheLocation(ctx context.Context, id string) (*file.Bucket, string, error) {
	bucket, prefix, err := remote.CacheDir(ctx, remote.SVOFilterProfile)
	if err != nil {
		return nil, "", fmt.Errorf("%w: cache: %w", ErrService, err)
	}

	// A filter identifier carries a slash, which is a key separator rather
	// than a character, so it is replaced instead of nesting the cache one
	// directory deep per photometric system.
	safe := strings.NewReplacer("/", "_", "\\", "_", " ", "_").Replace(id)

	return bucket, path.Join(prefix, safe+".xml"), nil
}

// Parse reads a VOTable filter profile.
//
// Exported because it is the half of this package that needs no network: a
// caller with a profile on disk, and every test here, goes through it.
//
// The document is walked as a token stream rather than decoded into a struct
// with a fixed element path. VOTable permits PARAM at more than one depth, and
// the service in fact puts them under RESOURCE>TABLE while a reasonable
// reading of the format puts them under RESOURCE. A parser pinned to one of
// those returns an empty metadata block for the other — silently, since a
// missing PARAM is indistinguishable from one that was never looked for.
func Parse(r io.Reader) (magnitude.Passband, error) {
	dec := xml.NewDecoder(r)

	param := map[string]string{}
	lambda := []unit.WavelengthNM{}
	response := []float64{}

	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return magnitude.Passband{}, fmt.Errorf("%w: %w", ErrService, err)
		}

		el, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		switch el.Name.Local {
		case "PARAM":
			name, value := attrs(el)

			// The service repeats some names; the first wins, which is the
			// one belonging to the filter rather than to a description block.
			if _, seen := param[name]; name != "" && !seen {
				param[name] = value
			}

		case "TR":
			var row struct {
				Cells []string `xml:"TD"`
			}

			if err := dec.DecodeElement(&row, &el); err != nil {
				return magnitude.Passband{}, fmt.Errorf("%w: row %d: %w",
					ErrService, len(lambda), err)
			}

			if len(row.Cells) < 2 {
				return magnitude.Passband{}, fmt.Errorf(
					"%w: row %d has %d columns, want wavelength and transmission",
					ErrService, len(lambda), len(row.Cells))
			}

			nm, err := strconv.ParseFloat(strings.TrimSpace(row.Cells[0]), 64)
			if err != nil {
				return magnitude.Passband{}, fmt.Errorf("%w: row %d wavelength: %w",
					ErrService, len(lambda), err)
			}

			t, err := strconv.ParseFloat(strings.TrimSpace(row.Cells[1]), 64)
			if err != nil {
				return magnitude.Passband{}, fmt.Errorf("%w: row %d transmission: %w",
					ErrService, len(lambda), err)
			}

			lambda = append(lambda, unit.WavelengthNM(nm/angstromPerNM))
			response = append(response, t)
		}
	}

	// The wavelengths are only in Angstrom because the service says so. A
	// profile in different units would be silently off by a factor of ten.
	if u := param["WavelengthUnit"]; u != "Angstrom" {
		return magnitude.Passband{}, fmt.Errorf(
			"%w: wavelengths are in %q, and only Angstrom is handled", ErrService, u)
	}

	detector, err := detectorOf(param["DetectorType"])
	if err != nil {
		return magnitude.Passband{}, err
	}

	zero, err := zeroPointJy(param)
	if err != nil {
		return magnitude.Passband{}, err
	}

	name := param["filterID"]
	if name == "" {
		name = "unnamed filter"
	}

	band := magnitude.Passband{
		Name:            name,
		WavelengthNM:    lambda,
		Response:        response,
		Detector:        detector,
		VegaZeroPointJy: zero,
	}

	if err := band.Validate(); err != nil {
		return magnitude.Passband{}, fmt.Errorf("%w: %q: %w", ErrService, name, err)
	}

	return band, nil
}

// detectorOf maps the service's DetectorType to the convention a passband
// integrates under.
//
// SVO documents 0 as an energy counter and 1 as a photon counter. There is no
// default: a profile that does not say is a profile whose integration
// convention is unknown, and guessing it tilts the answer across the band
// rather than shifting it, so it cannot be corrected for afterwards.
func detectorOf(v string) (magnitude.Detector, error) {
	switch strings.TrimSpace(v) {
	case "0":
		return magnitude.EnergyIntegrating, nil
	case "1":
		return magnitude.PhotonCounting, nil
	case "":
		return 0, fmt.Errorf("%w: the profile states no detector type", ErrService)
	default:
		return 0, fmt.Errorf("%w: detector type %q is neither 0 nor 1", ErrService, v)
	}
}

// zeroPointJy reads the Vega zero point.
//
// Both the unit and the magnitude system are checked rather than assumed. A
// zero point in erg/cm2/s/A would be off by orders of magnitude, and an AB
// zero point is a different system whose value happens to be plausible, which
// is the more dangerous of the two.
func zeroPointJy(param map[string]string) (float64, error) {
	if u := param["ZeroPointUnit"]; u != "Jy" {
		return 0, fmt.Errorf("%w: zero point is in %q, and only Jy is handled", ErrService, u)
	}

	if sys := param["MagSys"]; sys != "Vega" {
		return 0, fmt.Errorf(
			"%w: zero point is on the %q system, and Passband.VegaZeroPointJy is Vega",
			ErrService, sys)
	}

	v, err := strconv.ParseFloat(strings.TrimSpace(param["ZeroPoint"]), 64)
	if err != nil {
		return 0, fmt.Errorf("%w: zero point: %w", ErrService, err)
	}

	if v <= 0 {
		return 0, fmt.Errorf("%w: zero point is %g, which is not a flux density", ErrService, v)
	}

	return v, nil
}

// attrs pulls the name and value of a PARAM, whatever order they appear in.
func attrs(el xml.StartElement) (name, value string) {
	for _, a := range el.Attr {
		switch a.Name.Local {
		case "name":
			name = a.Value
		case "value":
			value = a.Value
		}
	}

	return name, value
}
