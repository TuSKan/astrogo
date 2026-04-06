// Package mast provides a remote provider for the Mikulski Archive for Space Telescopes.
//
// The mast package bridges the STScI Common Archive Observation Model (CAOM) REST endpoints
// dynamically resolving identifiers via the standard Mast.Name.Lookup module.
// Returned objects correspond to datasets from missions like Hubble (HST), James
// Webb Space Telescope (JWST), TESS, and Kepler natively parsed into ICRS targets.
package mast
