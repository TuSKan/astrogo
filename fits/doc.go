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
// # Conventions
//
// Both registered conventions that let a header carry what the 80-byte record
// cannot are implemented, in both directions:
//
//   - **HIERARCH** for keywords over eight characters, which ESO instruments
//     emit by the dozen per header.
//   - **CONTINUE** for string values over sixty-eight, announced by LONGSTRN,
//     as FITS 4.0 §4.2.1.2 and OGIP 100 define it.
//
// Unsigned integer images are stored as the signed type of the same width
// offset by BZERO = 2^(n-1) (FITS 4.0 §5.2.5), so a uint16 frame written here
// is a uint16 frame when read back, here or anywhere else.
//
// Every HDU carries DATASUM and CHECKSUM. The sum over a complete HDU comes out
// all ones, which is the property cfitsio's fits_verify_chksum and astropy's
// checksum verification apply, so a file written here verifies there.
//
// Null table values are carried rather than flattened: an integer column
// declares TNULLn and stores that sentinel, a floating-point column stores NaN,
// and both read back as nulls. A column whose data happens to contain its own
// sentinel declares none and keeps its values, since reserving a value that is
// really there would turn measurements into absences.
//
// Vector columns — a TFORM repeat above one, which is how a table carries a
// spectrum or a covariance row — read and write as fixed-size lists.
//
// # What the writer does not do
//
// **ASCII tables are not written**, and barely read: ReadASCIITable consumes
// the payload without decoding it. BINTABLE superseded the format decades ago
// and is what modern pipelines emit, so an ASCII table read here arrives as a
// header and a row count.
//
// The variable-length P and Q descriptors are not written either. A column
// declaring one is read as its raw stored values.
package fits
