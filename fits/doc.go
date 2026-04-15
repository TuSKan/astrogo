// Package fits provides I/O support for the Flexible Image Transport System (FITS)
// and the World Coordinate System (WCS).
//
// # FITS I/O
//
// The package reads standard FITS files via [Open] / [OpenMmap] and supports:
//   - Primary and extension HDUs (Image, BinTable, ASCII Table)
//   - Gzip-compressed streams (.fits.gz)
//   - Memory-mapped file access for zero-copy large-image workflows
//   - Apache Arrow columnar batch export for catalog-scale table HDUs
//
// # World Coordinate System
//
// The [WCS] type encodes the FITS standard pixel-to-sky mapping defined by
// CRPIX, CRVAL, CDELT, CTYPE, and the PC rotation matrix.
//
//   - [NewWCS] constructs an identity-mapped N-dimensional coordinate system.
//   - [WCS.PixelToWorld] implements the TAN (Gnomonic) spherical projection
//     and falls back to linear mapping for non-spherical axes.
//   - [ExtractWCS] populates a WCS directly from a FITS [Header].
package fits
