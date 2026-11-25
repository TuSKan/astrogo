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
// Nothing here knows what an endpoint is. Every method takes the base URL
// its request goes to, and remote resolves that through remote.URL(id)
// immediately before calling — so offline mode, Disable and SetURL apply to
// API calls exactly as they do to files, decided in the one package that
// owns them rather than reproduced down here.
//
// An API call is not a download, and the download policy is never asked
// about it — the request is the documented purpose of the method that makes
// it — with one exception: JPLHorizonsSPK, whose response carries a whole
// kernel, is registered as a separate endpoint from JPLHorizons so that the
// policy sees kernel generation without also seeing name resolution.
//
// This package is not importable from outside remote/. Reach it through
// remote.NewAPIClient.
//
// Transport is net/http, with each Client on a transport of its own. No
// net/http type appears in any signature here beyond the standard url.Values
// used to describe a query.
package api
