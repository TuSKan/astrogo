package atlas

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"log"

	gofs "github.com/ungerik/go-fs"

	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/skybrightness/lpmap"
)

// Names inside remote.WorldAtlas's archive (live-confirmed against the real
// ZIP central directory — see EnsureWorldAtlas's doc comment). The archive
// also carries a README.txt and a World_Atlas_2015.tpk tile package;
// neither is used here.
const (
	worldAtlasZipName  = "World_Atlas_2015.zip"
	worldAtlasZipEntry = "World_Atlas_2015.tif"
	// worldAtlasTIFFName is the extracted file's cache name — deliberately
	// the same as worldAtlasZipEntry, so the cache directory's contents are
	// self-explanatory.
	worldAtlasTIFFName = worldAtlasZipEntry

	// worldAtlasExtractedSize is World_Atlas_2015.tif's exact uncompressed
	// size in bytes, live-confirmed against the real archive's ZIP central
	// directory at implementation time. A local file of exactly this size
	// is trusted as a complete, already-extracted copy without re-running
	// the (still cheap, but unnecessary) decode+sample validation — unlike
	// EnsureVIIRSAnnual, which has no single fixed size to check against
	// since VIIRS archives vary year to year and grow over time.
	worldAtlasExtractedSize = 3_012_927_014
)

// validationLatDeg/LonDeg is central London — a bright, densely populated
// site guaranteed to carry real (non no-data) signal in every light-
// pollution source this file downloads. Every EnsureXxx function samples
// this point right after extraction purely to prove the file decodes; the
// sampled value itself is never inspected or compared against a reference.
const (
	validationLatDeg = 51.5074
	validationLonDeg = -0.1278
)

// ErrWorldAtlasCorrupt is returned when the extracted World Atlas GeoTIFF
// fails to decode or sample after extraction — the archive download or
// extraction was interrupted or corrupted. The partial/corrupt extracted
// file is removed before this error is returned, so a subsequent
// EnsureWorldAtlas call re-downloads and re-extracts rather than serving
// the broken copy forever.
var ErrWorldAtlasCorrupt = errors.New("atlas: extracted World Atlas GeoTIFF failed validation")

// config holds every Option's raw settings — shared by the download
// functions (EnsureWorldAtlas/OpenWorldAtlas/EnsureVIIRSAnnual/
// OpenVIIRSAnnual) and Resolver (see resolver.go). One flat option type
// for the whole package, rather than a separate type per entry point, is
// deliberate: it's what lets a Resolver forward a download-related Option
// straight through with no translation layer to wire up.
type config struct {
	keepArchive bool
	progress    func(downloaded, total int64)
	hasProgress bool
	cacheDir    string

	// The remaining fields matter only to Resolver (resolver.go); the
	// download functions in this file and download_viirs.go ignore them.
	layer Layer

	atlasFile    string
	hasAtlasFile bool

	viirsYear int
	// viirsYearPinned records that WithVIIRSYear named an explicit year,
	// which suppresses the newest-year probe (see Resolver.viirsYearFor).
	viirsYearPinned bool

	lpmapClient *lpmap.Client

	bortleClass int
	hasBortle   bool

	scalarSQM skybrightness.SurfaceBrightnessV
	hasScalar bool

	quiet bool
}

// Option configures EnsureWorldAtlas/OpenWorldAtlas, EnsureVIIRSAnnual/
// OpenVIIRSAnnual, and NewResolver.
type Option func(*config)

// WithKeepArchive controls whether the downloaded zip is kept on disk
// after a successful extraction. Default: false (deleted) — the extracted
// GeoTIFF alone is everything a provider needs, and keeping both means
// carrying the full combined footprint (roughly double the extracted
// size) indefinitely.
func WithKeepArchive(keep bool) Option {
	return func(c *config) { c.keepArchive = keep }
}

// WithDownloadProgress reports a download's progress (bytes downloaded,
// total bytes — total is 0 if unknown). Not called during extraction,
// which has no network activity to report against. See ProgressLogger
// for a ready-to-use implementation instead of hand-writing one; a
// Resolver applies ProgressLogger by default unless this or WithQuiet is
// given (see resolver.go).
func WithDownloadProgress(f func(downloaded, total int64)) Option {
	return func(c *config) { c.progress = f; c.hasProgress = true }
}

// ProgressLogger returns a WithDownloadProgress callback that logs one
// line to log.Default() every time the completed percentage crosses a
// new 10-point mark (10%, 20%, ...), not once per chunk — the ready-made
// default for "just show me it's working" without hand-writing percent
// arithmetic:
//
//	provider, closer, err := atlas.OpenWorldAtlas(ctx, atlas.WithDownloadProgress(atlas.ProgressLogger("World Atlas 2015")))
func ProgressLogger(label string) func(downloaded, total int64) {
	lastDecile := -1

	return func(downloaded, total int64) {
		if total <= 0 {
			return
		}

		pct := int(100 * downloaded / total)

		decile := pct / 10
		if decile == lastDecile {
			return
		}

		lastDecile = decile

		log.Printf("atlas: downloading %s... %3d%%", label, pct)
	}
}

// WithCacheDir overrides where the extracted GeoTIFF is written and read
// from, independent of astrogo's regular cache directory
// (remote.DataDir/remote.SetDataDir/ASTROGO_CACHE_DIR) — useful when the
// rest of astrogo's cache lives on a volume too small for these
// multi-gigabyte files. It does NOT relocate the downloaded zip itself:
// that download still goes through remote.GetFile, which always resolves
// the endpoint's own cache directory the normal way. Default: the same
// directory as the zip (remote.CacheDir(remote.WorldAtlas) /
// remote.CacheDir(remote.VIIRSAnnual) — both resolve to the same
// directory, since both endpoints share the "atlas" subsystem).
func WithCacheDir(dir string) Option {
	return func(c *config) { c.cacheDir = dir }
}

// EnsureWorldAtlas returns the local path to a fully extracted, validated
// copy of the Falchi et al. 2016 World Atlas 2015 GeoTIFF
// (World_Atlas_2015.tif, ~2.8 GB uncompressed artificial zenith sky
// brightness, mcd/m²), downloading and extracting the ~653 MB archive on
// first call. Subsequent calls are a fast, network-free no-op once a
// correctly-sized extracted copy exists locally.
//
// LICENSE: this dataset is CC BY-NC 4.0 (Attribution-NonCommercial),
// https://creativecommons.org/licenses/by-nc/4.0/ — non-commercial use
// only. Attribute Falchi, C.C.M., et al. (2016), "The new world atlas of
// artificial night sky brightness", Science Advances 2, e1600377. See
// remote.WorldAtlas's doc comment for the same notice.
//
// The archive download goes through remote.GetFile against
// remote.WorldAtlas, so it is gated exactly like every other bulk
// download in this library: call
// remote.EnableDownloads(remote.WorldAtlas, maxSize) (or
// remote.EnableAllDownloads) first, or this returns
// remote.ErrDownloadDenied. Peak local disk usage during the first call is
// roughly 3.5 GB (the downloaded zip plus the extracted TIFF); the zip is
// deleted after a successful, validated extraction unless
// WithKeepArchive(true) is given. There is no free-disk-space precheck —
// a genuine ENOSPC surfaces through the wrapped extraction error.
func EnsureWorldAtlas(ctx context.Context, opts ...Option) (string, error) {
	var cfg config
	for _, opt := range opts {
		opt(&cfg)
	}

	tiffDir, err := resolveExtractDir(cfg, remote.WorldAtlas)
	if err != nil {
		return "", err
	}

	tiffFile := tiffDir.Join(worldAtlasTIFFName)

	if tiffFile.Exists() && tiffFile.Size() == worldAtlasExtractedSize {
		return tiffFile.LocalPath(), nil
	}

	zipFile, err := remote.GetFile(ctx, remote.WorldAtlas, worldAtlasZipName, remote.WithProgress(cfg.progress))
	if err != nil {
		return "", fmt.Errorf("atlas: download World Atlas archive: %w", err)
	}

	if err := extractZIPEntry(zipFile, worldAtlasZipEntry, tiffFile); err != nil {
		return "", err
	}

	if err := validateExtractedWorldAtlas(tiffFile); err != nil {
		_ = tiffFile.Remove()

		return "", err
	}

	if !cfg.keepArchive {
		// Best-effort: a failed cleanup doesn't invalidate the already-
		// validated extracted copy this call is returning.
		_ = zipFile.Remove()
	}

	return tiffFile.LocalPath(), nil
}

// OpenWorldAtlas is EnsureWorldAtlas followed by opening the result as a
// windowed skybrightness.SQMProvider (see NewFalchiProvider) — the
// self-contained "give me a ready-to-query World Atlas provider" entry
// point most callers want. The returned io.Closer must be closed by the
// caller when done; it holds an open handle onto the ~2.8 GB extracted
// GeoTIFF, which is never read into memory in full — each query windows
// into just the tiles/strips it needs.
func OpenWorldAtlas(ctx context.Context, opts ...Option) (skybrightness.SQMProvider, io.Closer, error) {
	path, err := EnsureWorldAtlas(ctx, opts...)
	if err != nil {
		return nil, nil, err
	}

	rs, err := gofs.File(path).OpenReadSeeker()
	if err != nil {
		return nil, nil, fmt.Errorf("atlas: open %s: %w", path, err)
	}

	provider, err := NewFalchiProvider(rs)
	if err != nil {
		_ = rs.Close()

		return nil, nil, err
	}

	return provider, rs, nil
}

// resolveExtractDir resolves the directory an extracted GeoTIFF is written
// to/read from: cfg.cacheDir if WithCacheDir was given, otherwise the
// directory remote manages for id's own cache.
func resolveExtractDir(cfg config, id remote.EndpointID) (gofs.File, error) {
	if cfg.cacheDir != "" {
		return gofs.File(cfg.cacheDir), nil
	}

	dir, err := remote.CacheDir(id)
	if err != nil {
		return "", fmt.Errorf("atlas: %w", err)
	}

	return dir, nil
}

// extractZIPEntry extracts entryName from zipFile into destFile via
// remote.Save (atomic temp-file-then-rename on the local filesystem) —
// shared by every archive-based downloader in this file. Important for
// an interruptible, multi-gigabyte write: a killed process never leaves
// a partially-written file at destFile's final path for a later
// completeness check to mistake as complete.
func extractZIPEntry(zipFile gofs.File, entryName string, destFile gofs.File) error {
	rs, err := zipFile.OpenReadSeeker()
	if err != nil {
		return fmt.Errorf("atlas: open downloaded archive %s: %w", zipFile, err)
	}
	defer func() { _ = rs.Close() }()

	zr, err := zip.NewReader(rs, zipFile.Size())
	if err != nil {
		return fmt.Errorf("atlas: %s is not a valid zip archive: %w", zipFile, err)
	}

	entry, err := zr.Open(entryName)
	if err != nil {
		return fmt.Errorf("atlas: %s not found in %s: %w", entryName, zipFile, err)
	}
	defer func() { _ = entry.Close() }()

	if err := remote.Save(entry, destFile); err != nil {
		return fmt.Errorf("atlas: extract %s: %w", entryName, err)
	}

	return nil
}

// validateExtractedGeoTIFF reopens tiffFile and decodes+samples it at a
// known-bright site (central London), proving the file is a complete,
// well-formed GeoTIFF this package's reader can serve — shared by every
// archive-based downloader in this file. Callers wrap the returned error
// with their own sentinel (ErrWorldAtlasCorrupt, ErrVIIRSCorrupt).
func validateExtractedGeoTIFF(tiffFile gofs.File) error {
	rs, err := tiffFile.OpenReadSeeker()
	if err != nil {
		return fmt.Errorf("open %s: %w", tiffFile, err)
	}
	defer func() { _ = rs.Close() }()

	t, err := openGeoTIFF(rs, nil)
	if err != nil {
		return fmt.Errorf("decode %s: %w", tiffFile, err)
	}

	if _, err := t.sampleBilinear(validationLonDeg, validationLatDeg); err != nil {
		return fmt.Errorf("sample %s at validation site: %w", tiffFile, err)
	}

	return nil
}

// validateExtractedWorldAtlas is validateExtractedGeoTIFF wrapped with
// ErrWorldAtlasCorrupt, so a caller can errors.Is-check the World-Atlas-
// specific sentinel.
func validateExtractedWorldAtlas(tiffFile gofs.File) error {
	if err := validateExtractedGeoTIFF(tiffFile); err != nil {
		return fmt.Errorf("%w: %w", ErrWorldAtlasCorrupt, err)
	}

	return nil
}
