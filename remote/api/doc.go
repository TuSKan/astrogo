// Package api is astrogo's client for genuine request/response APIs: the
// services whose returned document shape depends on the query, not on a
// stable resource identity — SIMBAD, VizieR, Gaia, MAST, CelesTrak, FINK,
// JPL SBDB, and JPL Horizons.
//
// The dividing line against remote/file is what is being addressed, not
// which protocol carries it. A file on http is a file with an http
// backend, not an API: SPK kernels, IERS EOP bulletins, OpenNGC CSVs and
// GeoTIFF bundles are byte-addressable resources with a stable identity, a
// size, and range semantics, so they go through remote.GetFile whatever
// their scheme. A TAP query is not.
//
// Every call gates on remote.URL(id) first, so offline mode, Disable and
// SetURL apply here exactly as they do to files. An API call needs no
// download consent — the request is the documented purpose of the method
// that makes it — with one exception: JPLHorizonsSPK, whose response
// carries a whole kernel, is registered as a separate endpoint from
// JPLHorizons so that consent gates kernel generation without also gating
// name resolution.
//
// Transport is resty.dev/v3, wrapped completely: no resty type appears in
// any signature here, and neither does net/http beyond the standard
// url.Values used to describe a query. That keeps the transport
// replaceable, as it has already been replaced once.
package api
