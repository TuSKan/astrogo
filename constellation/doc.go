// Package constellation identifies which of the 88 IAU constellations a
// given ICRS sky position falls in.
//
// # Data source and licensing
//
// The boundary data (boundary_data.go) transcribes the raw right
// ascension/declination/constellation values from CDS/VizieR catalog
// VI/49 ("Constellation Boundary Data", Davenhall & Leggett 1989), file
// bound_18.dat — https://vizier.cfa.harvard.edu/ftp/cats/VI/49/bound_18.dat.
// That catalog compiles the official IAU constellation boundaries Delporte
// defined in 1930. It's public astronomical reference data, not
// third-party software; nothing here is derived from, or copies, any
// other project's source code or specific compact encoding of this data —
// Lookup implements the standard even-odd ray-casting point-in-polygon
// test (see Roman, N.G. 1987, PASP 99, 695 for the original published
// algorithm this package's approach is equivalent to) directly against
// the raw boundary vertices.
//
// # Why precession matters here
//
// Delporte's boundaries are defined at the B1875.0 equinox. Lookup
// precesses its input from J2000 to B1875 (via the IAU 1976 precession
// model) before testing — comparing a J2000 position directly against
// B1875 boundaries would be wrong by the roughly 1.4° of precessional
// drift accumulated since 1875, enough to misclassify stars near a
// boundary.
package constellation
