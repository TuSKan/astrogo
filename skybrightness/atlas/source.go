package atlas

import (
	"errors"
	"fmt"
	"io"

	"github.com/TuSKan/astrogo/skybrightness"
)

// ErrUnknownSource is returned for an unrecognized [Source].
var ErrUnknownSource = errors.New("atlas: unknown source")

// ErrSourceUnavailable is returned when a known [Source] has no obtainable
// numeric data (currently [LightPollutionAtlas]; see [NewLorenzProvider]).
var ErrSourceUnavailable = errors.New("atlas: source has no obtainable numeric data")

// Source identifies a published light-pollution atlas, mirroring the "pick your
// layer" catalog exposed by mainstream light-pollution tools. The data vintage
// is carried in [SourceInfo.Year], not the identifier.
type Source int

const (
	// WorldAtlas is the Falchi et al. 2016 "World Atlas 2015" — propagated
	// artificial zenith sky brightness (mcd/m²). The default, highest-fidelity
	// numeric floor. Loaded by [NewFalchiProvider].
	WorldAtlas Source = iota
	// LightPollutionAtlas is the Lorenz Light Pollution Atlas (annual through
	// 2024/2025) — propagated, freshest, but published only as PNG zone maps, so
	// it currently has no obtainable numeric grid (see [NewLorenzProvider]).
	LightPollutionAtlas
	// VIIRS is a VIIRS annual radiance composite (VNP46A4/VJ146A4, EOG VNL) — raw
	// upward radiance (nW·cm⁻²·sr⁻¹), fresh and downloadable but NOT propagated;
	// converted by an empirical fit at lower fidelity. Loaded by
	// [NewVIIRSProvider] (GeoTIFF) / [NewVIIRSHDF5Provider] (HDF5).
	VIIRS
)

// SourceInfo is descriptive metadata for a [Source], suitable for presenting a
// selectable catalog (name, vintage, fidelity, quantity, availability).
type SourceInfo struct {
	Source Source
	// Short is the compact label (e.g. "WA-2015").
	Short string
	// Name is the full descriptive name.
	Name string
	// Year is the data vintage.
	Year int
	// Propagated is true for radiative-transfer sky brightness (WorldAtlas,
	// LightPollutionAtlas) and false for raw radiance converted by an empirical
	// fit (VIIRS).
	Propagated bool
	// Fidelity ranks sources, 1 = highest. WorldAtlas and LightPollutionAtlas are
	// 1; VIIRS is 2.
	Fidelity int
	// Quantity describes the stored physical quantity and units.
	Quantity string
	// Available is false when no numeric grid is currently obtainable
	// (LightPollutionAtlas).
	Available bool
	// Reference is the DOI or source URL.
	Reference string
}

// sourceCatalog is the ordered metadata table for all known sources.
var sourceCatalog = [...]SourceInfo{
	WorldAtlas: {
		Source: WorldAtlas, Short: "WA-2015", Name: "World Atlas 2015 (Falchi et al. 2016)",
		Year: 2015, Propagated: true, Fidelity: 1,
		Quantity: "artificial zenith sky brightness (mcd/m²)", Available: true,
		Reference: "doi:10.5880/GFZ.1.4.2016.001",
	},
	LightPollutionAtlas: {
		Source: LightPollutionAtlas, Short: "LPA-2024", Name: "Lorenz Light Pollution Atlas 2024",
		Year: 2024, Propagated: true, Fidelity: 1,
		Quantity: "artificial zenith sky brightness (mcd/m²)", Available: false,
		Reference: "djlorenz.github.io/astronomy/lp/",
	},
	VIIRS: {
		Source: VIIRS, Short: "VIIRS", Name: "VIIRS annual radiance composite (VNP46A4/VJ146A4)",
		Year: 2025, Propagated: false, Fidelity: 2,
		Quantity: "upward radiance (nW·cm⁻²·sr⁻¹)", Available: true,
		Reference: "doi:10.1038/s41598-020-64673-2",
	},
}

// Sources returns the catalog of known atlas sources, in fidelity-then-vintage
// order (WorldAtlas, LightPollutionAtlas, VIIRS).
func Sources() []SourceInfo {
	out := make([]SourceInfo, len(sourceCatalog))
	copy(out, sourceCatalog[:])

	return out
}

// Info returns the descriptive metadata for the source.
func (s Source) Info() (SourceInfo, error) {
	if int(s) < 0 || int(s) >= len(sourceCatalog) {
		return SourceInfo{}, fmt.Errorf("%w: %d", ErrUnknownSource, int(s))
	}

	return sourceCatalog[s], nil
}

// String implements [fmt.Stringer], returning the short label.
func (s Source) String() string {
	if info, err := s.Info(); err == nil {
		return info.Short
	}

	return fmt.Sprintf("Source(%d)", int(s))
}

// OpenGeoTIFF builds an [skybrightness.SQMProvider] for a GeoTIFF-backed source,
// dispatching on src: [WorldAtlas] → [NewFalchiProvider], [VIIRS] →
// [NewVIIRSProvider]. [LightPollutionAtlas] returns [ErrSourceUnavailable] (no
// numeric grid). gt, when non-nil, supplies the affine geotransform for files
// lacking GeoTIFF model tags.
//
// This is the convenience selector for the common GeoTIFF case; for HDF5 VIIRS
// granules, per-source options, or coefficient overrides, call the specific
// constructor directly.
func OpenGeoTIFF(src Source, r io.ReaderAt, gt *GeoTransform) (skybrightness.SQMProvider, error) {
	switch src {
	case WorldAtlas:
		if gt != nil {
			return NewFalchiProvider(r, WithGeoTransform(*gt))
		}

		return NewFalchiProvider(r)
	case VIIRS:
		if gt != nil {
			return NewVIIRSProvider(r, WithVIIRSGeoTransform(*gt))
		}

		return NewVIIRSProvider(r)
	case LightPollutionAtlas:
		return nil, fmt.Errorf("%w: %s (%w)", ErrSourceUnavailable, src, ErrLorenzNoNumericData)
	default:
		return nil, fmt.Errorf("%w: %d", ErrUnknownSource, int(src))
	}
}
