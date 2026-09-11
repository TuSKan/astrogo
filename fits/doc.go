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
// [Write] encodes a [File] back out: images at every BITPIX, and binary tables
// built from an Arrow batch. It takes an [io.Writer] rather than a path, so a
// file, a bucket, a socket or a buffer are all the same call.
//
// The structural keywords — BITPIX, NAXIS, NAXISn, NAXIS1/2, TFIELDS, TFORMn —
// are derived from the data being written rather than copied from the stored
// header, because the two disagree the moment a caller filters a table's rows
// or replaces an image's pixels. A file whose header describes something it
// does not contain is unreadable in a way no reader can diagnose.
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
