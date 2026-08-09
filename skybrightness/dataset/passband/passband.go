package passband

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	gofs "github.com/ungerik/go-fs"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/unit"
)

// ErrInvalidBundle is returned by OpenBundle for a malformed manifest or
// curve file.
var ErrInvalidBundle = errors.New("passband: invalid bundle")

type manifestCurve struct {
	ID              string  `json:"id"`
	File            string  `json:"file"`
	System          string  `json:"system"`
	Detector        string  `json:"detector"`
	Checksum        string  `json:"checksum"`
	VegaMeanFlambda float64 `json:"vega_mean_flambda_w_m2_sr_nm"`
	VegaSpectrum    string  `json:"vega_spectrum"`
	VegaUncertainty float64 `json:"vega_uncertainty_mag"`
	Source          string  `json:"source"`
	Licence         string  `json:"licence"`
}

type manifest struct {
	Version string          `json:"version"`
	Curves  []manifestCurve `json:"curves"`
}

// OpenBundle reads a versioned, checksummed passband bundle directory
// (see doc.go for the format) and returns it as a skybrightness.PassbandSet.
// No network access; a caller-supplied or already-extracted directory.
//
// Reads through gofs.File, matching this codebase's own methodology
// (remote/file.go and Remote below) rather than raw os.ReadFile/os.Open.
func OpenBundle(dir string) (skybrightness.PassbandSet, error) {
	manifestFile := gofs.File(dir).Join("manifest.json")

	raw, err := manifestFile.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%w: read manifest: %w", ErrInvalidBundle, err)
	}

	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("%w: parse manifest: %w", ErrInvalidBundle, err)
	}

	pbs := make([]*skybrightness.Passband, 0, len(m.Curves))

	for _, c := range m.Curves {
		pb, err := loadCurve(dir, c)
		if err != nil {
			return nil, fmt.Errorf("%w: curve %q: %w", ErrInvalidBundle, c.ID, err)
		}

		pbs = append(pbs, pb)
	}

	return skybrightness.NewPassbandSet(atmosphere.DatasetVersion(m.Version), pbs...), nil
}

func loadCurve(dir string, c manifestCurve) (*skybrightness.Passband, error) {
	f := gofs.File(dir).Join(c.File)

	r, err := f.OpenReader()
	if err != nil {
		return nil, fmt.Errorf("open curve file: %w", err)
	}
	defer r.Close() //nolint:errcheck // read-only, nothing to flush

	wl, resp, err := parseCurveCSV(r)
	if err != nil {
		return nil, err
	}

	pb := &skybrightness.Passband{
		ID:         skybrightness.PassbandID(c.ID),
		System:     parseSystem(c.System),
		Detector:   parseDetector(c.Detector),
		Wavelength: wl,
		Response:   resp,
		Version:    atmosphere.DatasetVersion(c.Checksum),
		Source: atmosphere.SourceRef{
			Name: c.Source, Checksum: c.Checksum, Licence: c.Licence,
			Fidelity: atmosphere.FidelityMeasured,
		},
	}

	if c.VegaMeanFlambda > 0 {
		pb.VegaZP = &skybrightness.VegaZeroPoint{
			MeanFlambda: unit.SpectralRadiance(c.VegaMeanFlambda),
			Spectrum:    c.VegaSpectrum,
			Uncertainty: c.VegaUncertainty,
		}
	}

	if err := pb.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidBundle, err)
	}

	return pb, nil
}

// parseCurveCSV streams the curve CSV row by row via cr.Read(), rather than
// cr.ReadAll() (which buffers the whole file into a [][]string at once) —
// bounded memory regardless of curve length, matching how extractTarGz
// above already streams tar entries instead of buffering the archive.
func parseCurveCSV(r io.Reader) ([]unit.WavelengthNM, []float64, error) {
	cr := csv.NewReader(r)

	if _, err := cr.Read(); err != nil { // header row; columns unused, only its presence matters
		if errors.Is(err, io.EOF) {
			return nil, nil, fmt.Errorf("%w: curve CSV needs a header row and >= 1 data row", ErrInvalidBundle)
		}

		return nil, nil, fmt.Errorf("%w: read curve CSV header: %w", ErrInvalidBundle, err)
	}

	var wl []unit.WavelengthNM

	var resp []float64

	for {
		row, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return nil, nil, fmt.Errorf("%w: read curve CSV row: %w", ErrInvalidBundle, err)
		}

		if len(row) < 2 {
			continue
		}

		w, err := strconv.ParseFloat(strings.TrimSpace(row[0]), 64)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: bad wavelength %q", ErrInvalidBundle, row[0])
		}

		v, err := strconv.ParseFloat(strings.TrimSpace(row[1]), 64)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: bad response %q", ErrInvalidBundle, row[1])
		}

		wl = append(wl, unit.WavelengthNM(w))
		resp = append(resp, v)
	}

	if len(wl) == 0 {
		return nil, nil, fmt.Errorf("%w: curve CSV needs a header row and >= 1 data row", ErrInvalidBundle)
	}

	return wl, resp, nil
}

func parseSystem(s string) skybrightness.MagSystem {
	switch s {
	case "AB":
		return skybrightness.SystemAB
	case "Vega":
		return skybrightness.SystemVega
	default:
		return skybrightness.SystemPhotometricNone
	}
}

func parseDetector(s string) skybrightness.DetectorType {
	if s == "EnergyIntegrating" {
		return skybrightness.EnergyIntegrating
	}

	return skybrightness.PhotonCounting
}

// Remote fetches the versioned passband bundle via remote.PassbandBundle
// (consent-gated, KindFile), extracts it into remote's cache directory if
// not already extracted, and returns OpenBundle's result. No bundle is
// published to that endpoint as of this module's Phase 1 (see doc.go) —
// this function is structurally complete and will work once one is, per
// this codebase's convention of shipping the download/extract mechanism
// ahead of the dataset it targets (see skybrightness/dataset/worldatlas's
// own history for precedent).
func Remote(ctx context.Context, opts ...remote.ReadOption) (skybrightness.PassbandSet, error) {
	cacheDir, err := remote.CacheDir(remote.PassbandBundle)
	if err != nil {
		return nil, fmt.Errorf("passband: cache dir: %w", err)
	}

	extractedDir := cacheDir.Join("extracted")

	if !extractedDir.Exists() {
		archive, err := remote.GetFile(ctx, remote.PassbandBundle, "passbands-v1.tar.gz", opts...)
		if err != nil {
			return nil, fmt.Errorf("passband: download: %w", err)
		}

		r, err := archive.OpenReader()
		if err != nil {
			return nil, fmt.Errorf("passband: open archive: %w", err)
		}
		defer r.Close() //nolint:errcheck // read-only

		if err := extractTarGz(r, extractedDir.LocalPath()); err != nil {
			return nil, fmt.Errorf("passband: extract: %w", err)
		}
	}

	return OpenBundle(extractedDir.LocalPath())
}

func extractTarGz(r io.Reader, destDir string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("%w: gzip: %w", ErrInvalidBundle, err)
	}
	defer gz.Close() //nolint:errcheck // read-only

	tr := tar.NewReader(gz)

	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return fmt.Errorf("mkdir %s: %w", destDir, err)
	}

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil {
			return fmt.Errorf("%w: tar: %w", ErrInvalidBundle, err)
		}

		target := filepath.Join(destDir, filepath.Clean(hdr.Name))
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("%w: archive entry %q escapes destination", ErrInvalidBundle, hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o750); err != nil {
				return fmt.Errorf("mkdir %s: %w", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				return fmt.Errorf("mkdir %s: %w", filepath.Dir(target), err)
			}

			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) //nolint:gosec // path validated above
			if err != nil {
				return fmt.Errorf("create %s: %w", target, err)
			}

			_, copyErr := io.Copy(out, tr) //nolint:gosec // tar entry sizes are bounded by the archive's own declared size, not attacker-amplifiable beyond it
			closeErr := out.Close()

			if copyErr != nil {
				return fmt.Errorf("write %s: %w", target, copyErr)
			}

			if closeErr != nil {
				return fmt.Errorf("close %s: %w", target, closeErr)
			}
		}
	}
}
