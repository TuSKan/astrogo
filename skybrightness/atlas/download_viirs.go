package atlas

import (
	"context"
	"errors"
	"fmt"
	"io"

	gofs "github.com/ungerik/go-fs"

	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness"
)

// EarliestVIIRSYear/LatestVIIRSYear bound the annual composite years
// remote.VIIRSAnnual is KNOWN to serve, live-confirmed at implementation
// time (2026-08-01). LatestVIIRSYear is a floor, not a ceiling: upstream
// publishes a new year annually, so [NewestVIIRSYear] probes forward from
// here to find what is actually available rather than letting this
// constant silently go stale.
const (
	EarliestVIIRSYear = 2012
	LatestVIIRSYear   = 2025

	// maxVIIRSProbeAhead caps how many years past LatestVIIRSYear
	// NewestVIIRSYear will probe before giving up. Upstream adds one year
	// at a time, so this only has to cover how far behind this constant
	// is allowed to drift — it is a runaway guard, not a real limit.
	maxVIIRSProbeAhead = 5
)

// NewestVIIRSYear reports the newest annual composite remote.VIIRSAnnual
// actually publishes, by probing forward from [LatestVIIRSYear] until a
// year is missing. Probing uses HEAD requests, which transfer no body and
// so need no download consent (see [remote.Exists]).
//
// It is deliberately forgiving: a probe failure (offline, unreachable
// host, a 5xx) returns the best year confirmed so far — never worse than
// [LatestVIIRSYear] — alongside the error, so a caller that ignores the
// error still gets a usable year rather than zero. Costs one request in
// the common case where this constant is current.
func NewestVIIRSYear(ctx context.Context) (int, error) {
	newest := LatestVIIRSYear

	for year := LatestVIIRSYear + 1; year <= LatestVIIRSYear+maxVIIRSProbeAhead; year++ {
		ok, err := remote.Exists(ctx, remote.VIIRSAnnual, viirsZipName(year))
		if err != nil {
			return newest, fmt.Errorf("atlas: probe VIIRS %d: %w", year, err)
		}

		if !ok {
			break
		}

		newest = year
	}

	return newest, nil
}

// ErrVIIRSYearOutOfRange is returned for a year before
// [EarliestVIIRSYear], when no VIIRS composite can exist. There is
// deliberately no upper bound — see EnsureVIIRSAnnual.
var ErrVIIRSYearOutOfRange = errors.New("atlas: VIIRS year out of range")

// ErrVIIRSCorrupt is returned when an extracted VIIRS annual-composite
// GeoTIFF fails to decode or sample after extraction — see
// ErrWorldAtlasCorrupt for the equivalent World Atlas case; the same
// remove-and-retry behavior applies here.
var ErrVIIRSCorrupt = errors.New("atlas: extracted VIIRS GeoTIFF failed validation")

// viirsZipName/viirsZipEntry name remote.VIIRSAnnual's per-year archive
// and the single GeoTIFF entry inside it — live-confirmed against the
// real ZIP central directory for the 2025 archive at implementation time
// (viirs_2025_raw.zip -> viirs_2025_raw.tif); every other year follows
// the same lightpollutionmap.info naming convention documented at
// remote.VIIRSAnnual.
func viirsZipName(year int) string  { return fmt.Sprintf("viirs_%d_raw.zip", year) }
func viirsZipEntry(year int) string { return fmt.Sprintf("viirs_%d_raw.tif", year) }

// validateExtractedVIIRS is validateExtractedGeoTIFF wrapped with
// ErrVIIRSCorrupt, so a caller can errors.Is-check the VIIRS-specific
// sentinel — mirrors validateExtractedWorldAtlas in download.go.
func validateExtractedVIIRS(tiffFile gofs.File) error {
	if err := validateExtractedGeoTIFF(tiffFile); err != nil {
		return fmt.Errorf("%w: %w", ErrVIIRSCorrupt, err)
	}

	return nil
}

// EnsureVIIRSAnnual returns the local path to a fully extracted, validated
// copy of one year's VIIRS annual raw-radiance composite (nW·cm⁻²·sr⁻¹,
// "AllAngle_Composite_Snow_Free"), downloading and extracting the
// ~700 MB-1 GB archive on first call for that year. Subsequent calls for
// the same year are a fast, network-free no-op once a valid extracted
// copy exists locally.
//
// Unlike EnsureWorldAtlas, completeness is checked by re-validating the
// extracted file (decode + sample) rather than comparing against one
// fixed expected size — VIIRS archive sizes vary year to year and can
// grow when a year is reprocessed (see remote.VIIRSAnnual's Mutable note).
//
// LICENSE: the underlying VIIRS/Black Marble data is CC0 (public domain).
// The mirror itself asks that use be credited to "Jurij Stare,
// www.lightpollutionmap.info" plus "NASA's Black Marble nighttime lights
// product" — see remote.VIIRSAnnual's doc comment for the exact wording.
//
// The archive download goes through remote.GetFile against
// remote.VIIRSAnnual, gated exactly like every other bulk download in
// this library: call remote.EnableDownloads(remote.VIIRSAnnual, maxSize)
// (or remote.EnableAllDownloads) first, or this returns
// remote.ErrDownloadDenied. The zip is deleted after a successful,
// validated extraction unless WithKeepArchive(true) is given.
func EnsureVIIRSAnnual(ctx context.Context, year int, opts ...Option) (string, error) {
	// Lower bound only. Bounding this above by LatestVIIRSYear would
	// defeat NewestVIIRSYear entirely: the moment upstream publishes a
	// year past the compiled-in constant, the probe would find it and
	// this guard would then refuse to fetch it. Upstream is authoritative
	// on which years exist — a year it does not carry surfaces as a
	// download error, which is both accurate and self-updating.
	if year < EarliestVIIRSYear {
		return "", fmt.Errorf("%w: %d (VIIRS composites start at %d)", ErrVIIRSYearOutOfRange, year, EarliestVIIRSYear)
	}

	var cfg config
	for _, opt := range opts {
		opt(&cfg)
	}

	tiffDir, err := resolveExtractDir(cfg, remote.VIIRSAnnual)
	if err != nil {
		return "", err
	}

	entryName := viirsZipEntry(year)
	tiffFile := tiffDir.Join(entryName)

	if tiffFile.Exists() && validateExtractedVIIRS(tiffFile) == nil {
		return tiffFile.LocalPath(), nil
	}

	zipFile, err := remote.GetFile(ctx, remote.VIIRSAnnual, viirsZipName(year), remote.WithProgress(cfg.progress))
	if err != nil {
		return "", fmt.Errorf("atlas: download VIIRS %d archive: %w", year, err)
	}

	if err := extractZIPEntry(zipFile, entryName, tiffFile); err != nil {
		return "", err
	}

	if err := validateExtractedVIIRS(tiffFile); err != nil {
		_ = tiffFile.Remove()

		return "", err
	}

	if !cfg.keepArchive {
		_ = zipFile.Remove()
	}

	return tiffFile.LocalPath(), nil
}

// OpenVIIRSAnnual is EnsureVIIRSAnnual followed by opening the result as a
// windowed skybrightness.SQMProvider via NewVIIRSProvider — see
// OpenWorldAtlas for the equivalent World Atlas convenience. The returned
// io.Closer must be closed by the caller when done.
//
// FIDELITY WARNING (inherited from NewVIIRSProvider): VIIRS is raw
// radiance converted by an empirical fit, not propagated through an
// atmospheric radiative-transfer model like the World Atlas — prefer
// EnsureWorldAtlas/OpenWorldAtlas for fidelity, VIIRS for freshness
// (data through LatestVIIRSYear) and year-to-year trend comparisons.
func OpenVIIRSAnnual(ctx context.Context, year int, opts ...Option) (skybrightness.SQMProvider, io.Closer, error) {
	path, err := EnsureVIIRSAnnual(ctx, year, opts...)
	if err != nil {
		return nil, nil, err
	}

	rs, err := gofs.File(path).OpenReadSeeker()
	if err != nil {
		return nil, nil, fmt.Errorf("atlas: open %s: %w", path, err)
	}

	provider, err := NewVIIRSProvider(rs)
	if err != nil {
		_ = rs.Close()

		return nil, nil, err
	}

	return provider, rs, nil
}
