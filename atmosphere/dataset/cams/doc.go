// Package cams is a minimal, read-only NetCDF-4/HDF5 reader for CAMS
// (Copernicus Atmosphere Monitoring Service) global analysis files —
// the Sky Brightness V2 Phase 3/7 building block docs/skybrightness.md
// §8's "CAMS aerosol data" notes describe, and the reader
// atmosphere.Atmosphere's future live, geographically-resolved aerosol
// tier is meant to sit on top of (the OPAC-sourced
// atmosphere.RuralAerosol/UrbanAerosol/DesertAerosol/MaritimeAerosol
// presets remain the offline, zero-dependency default; this package is
// the operational counterpart, not a replacement).
//
// # Access
//
// Files are fetched via remote.GetFile against remote.CopernicusEODATA,
// using remote/s3 as the transport (see that package's doc comment for
// the credential contract — this package never reads a credential file
// or an S3 key itself, and knows nothing about S3 at all). Open reads
// bucket/key through bucket.NewReader rather than assuming a particular
// backend: bucket is normally the local cache remote.GetFile already
// produced, but this package never special-cases that — the same Open
// call works for any *file.Bucket (local, S3, ...), since scigolib/hdf5's
// on-disk format needs true random access to the file it decodes,
// forcing a scratch temp-file staging step Open performs generically
// (see Open's own doc comment for exactly why and how).
//
// # Why github.com/scigolib/hdf5, and what was verified before using it
//
// astrogo used to depend on this library (removed when skybrightness/atlas
// was deleted; see docs/reports/skybrightness-phase1.md). Re-adopting it
// here was gated on a live decision-gate spike against the real CAMS
// files this package targets — not assumed to still work, and not
// assumed to work at all for a 4-dimensional, chunked, deflate-compressed
// NetCDF-4 layout the library's own docs don't mention by name:
//
//  1. N-dimensional shape reporting. The public API has no structured
//     Dims()/Shape() method — Dataset.Info() returns only a formatted
//     string. This package does not parse that string: it derives shape
//     from real per-file NetCDF-4 metadata instead (see "How shape and
//     axis order are discovered" below), which is more robust and,
//     unlike Info()'s string format, is a stable third-party convention
//     this library didn't invent.
//  2. Chunked + deflate reads. Confirmed live: Dataset.Read() correctly
//     decoded a full 182 MB, 4-dimensional, chunk=[1,1,451,900]
//     deflate-level-1 aerosol tracer file (55,608,300 float64 values,
//     exact count, physically plausible values). Dataset.ReadSlice/
//     ReadHyperslab additionally do real chunk-selective I/O — confirmed
//     by reading readHyperslabChunked's implementation, not just its doc
//     comment — making Var.ReadPlane below ~86x faster than a full decode
//     for the common "one time/level plane" access pattern (29ms vs.
//     2.48s in the live spike). A single vertical column is still
//     expensive (~2.2s, since it touches all 137 level chunks) — this
//     matches, and does not solve, the point-query-hostile chunk layout
//     docs/skybrightness.md §8 already documents; Var.At's plane cache
//     only helps repeated queries within one time/level plane.
//  3. Attribute reads. units/long_name/_FillValue/missing_value all
//     decode correctly and were cross-checked against ncdump -h's own
//     output for the same real files (_FillValue = 9.969209968386869e+36,
//     matching the documented NC_FILL_DOUBLE value to full double
//     precision).
//
// Two real, separate limitations were found while building this
// package's own synthetic test fixture (reader_test.go) — not from the
// read-side spike above, and not affecting this package's real read
// path against actual downloaded CAMS files:
//
//   - scigolib/hdf5 v0.14.0's write path for a dataset that is both
//     chunked (WithChunkDims) AND carries attributes (WriteAttribute) is
//     broken: writing attributes onto an already-chunked dataset
//     corrupts the on-disk Data Layout message, and reading it back
//     fails with "failed to parse layout: chunked layout dimension N
//     truncated (32-bit)". Confirmed isolated: a chunked dataset with
//     zero attributes round-trips fine; the identical dataset with a
//     handful of attributes attached does not. Plain contiguous layout
//     with the exact same attributes round-trips correctly, including
//     through ReadSlice.
//   - WithGZIPCompression's write path is separately broken: even with
//     zero other attributes, a gzip-compressed chunked dataset fails on
//     read with "unsupported filter ID: 0" — not even gzip's real filter
//     ID, a write-side filter-pipeline encoding bug.
//
// Neither bug is exercised by this package's real read path — the
// actual chunked, gzip-compressed, attribute-bearing CAMS files this
// reader targets were independently confirmed to decode correctly
// (point 2 above), because this package only ever reads files scigolib/
// hdf5 did not write itself. reader_test.go's synthetic fixture
// therefore uses plain (uncompressed, unchunked) contiguous layout,
// which round-trips correctly and is sufficient to validate this
// package's shape/axis/fill-value/plane-extraction logic — the real
// storage layout's I/O-cost implications were already established
// against production data, not something an offline unit test needs to
// re-prove.
//
// This makes atmosphere/dataset/cams a second, independent importer of
// github.com/scigolib/hdf5 alongside the (not yet built)
// skybrightness/dataset/granule — CLAUDE.md's and docs/skybrightness.md's
// "the ONLY importer" language describes that package's role *within
// skybrightness*, not a repo-wide exclusivity; two unrelated dataset
// packages in different subtrees are free to depend on the same pure-Go
// decoder independently.
//
// # How shape and axis order are discovered
//
// Rather than parsing Dataset.Info()'s string, this package reads real
// NetCDF-4-on-HDF5 metadata directly, confirmed live against every real
// CAMS file this package was built against:
//
//   - A dimension-scale dataset (one of longitude/latitude/level/time)
//     carries a CLASS="DIMENSION_SCALE" attribute and a _Netcdf4Dimid
//     int32 attribute naming its dimension ID. Its own length (from
//     Dataset.Read()) is that dimension's size — this is File.Dims().
//   - A data variable (lnsp, den, aermrNN, ...) carries a
//     _Netcdf4Coordinates []int32 attribute listing its axes' dimension
//     IDs in on-disk order. Mapping each ID back to the dimension-scale
//     dataset that declared it gives the variable's real axis names and
//     order — confirmed live to be (time, latitude, longitude) for lnsp
//     (3 IDs) and (time, level, latitude, longitude) for aermr01/den
//     (4 IDs), matching docs/skybrightness.md §8's documented convention
//     exactly, but derived from the file's own metadata rather than
//     assumed.
//
// # Scope
//
// One file, one data variable (the real CAMS convention: "one variable
// per file") — Open reads every dimension-scale dataset eagerly (they are
// tiny: at most a few hundred elements) and indexes data variables by
// name without reading their bulk data. Var.ReadPlane/Var.At are the only
// bulk-data entry points, and both apply fill-value substitution (NaN) at
// the read boundary — a caller never sees the raw sentinel value.
// Tracer availability is dataset/version-specific (docs/skybrightness.md
// §8): File.Var returns ErrVariableNotFound for a variable the file
// doesn't have — distinguishable via errors.Is from any other error,
// which means the variable exists but its metadata could not be decoded
// — never a fabricated zero-filled Var.
//
// Explicitly out of scope, deliberately not built here: the ECMWF L137
// hybrid A/B pressure-reconstruction formula (real pressure from
// ps=exp(lnsp) and the model levels' half-level coefficients) and the
// aermr-tracer → species/bin → PSD → refractive-index → MOPSMAP optical
// mapping — both recorded as the next scientific work in
// docs/skybrightness.md §8, not this package's job.
package cams
