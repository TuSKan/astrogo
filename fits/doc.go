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
// # What the writer does not do
//
// The output is standard FITS and interoperable — the card layout is the fixed
// format of FITS 4.0 §4.1.2, EXTEND announces extensions, and every structure
// is block-aligned — but it is a smaller writer than astropy's, and these are
// the differences worth knowing before reaching for one:
//
//   - **No CONTINUE long strings and no HIERARCH.** A keyword over eight
//     characters, or a value and comment that together overflow the 80-byte
//     record, is [ErrCardTooLong] rather than a convention-encoded card.
//     Both conventions are legal and widely read; neither is implemented, so
//     a long instrument keyword has nowhere to go.
//   - **No unsigned integer images.** FITS expresses uint16 as int16 with
//     BZERO 32768, and this refuses rather than doing it silently, because
//     the values would otherwise read back negative anywhere the BZERO is
//     ignored.
//   - **No CHECKSUM or DATASUM.** The functions exist ([CalcChecksum],
//     [ValidateDatasum]) and the writer does not call them.
//   - **No TNULLn.** A null in a table column is written as the type's zero.
//     FITS has no null bitmap, and inventing a TNULLn would reserve a value
//     that might be real data.
//   - **Scalar table columns only**, plus fixed-width strings. Vector columns
//     (TFORM repeat above one), TDIMn and the variable-length P/Q descriptors
//     are not written — which matches the reader, since it does not decode
//     them either.
//   - **No ASCII table writing.** BINTABLE is what modern pipelines use; an
//     ASCII table read in cannot be written back out.
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
