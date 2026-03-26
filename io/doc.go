// Package astroio provides generic I/O utilities for astrogo data formats.
//
// The import path is github.com/TuSKan/astrogo/io; the package name is
// "astroio" to avoid shadowing the standard library "io" package.
//
// astroio owns:
//   - streaming readers/writers for row-oriented catalog formats,
//   - format detection (magic bytes, extension hints),
//   - common reader/writer interfaces shared by fits, catalog, and ephemeris
//     loader packages.
//
// Status: placeholder — implementation not yet started.
package astroio
