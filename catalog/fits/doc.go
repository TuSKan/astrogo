// Package fits provides a catalog [resolve.Provider] implementation backed by
// local FITS files.
//
// It reads object tables from FITS binary-table HDUs, parses ICRS coordinates,
// and exposes them through the standard [resolve.Provider] interface for name
// resolution and substring search.
package fits
