// Package mast resolves object names through the Mikulski Archive for Space
// Telescopes' name lookup, the Mast.Name.Lookup service, which relays a query
// to NED or SIMBAD and answers with the object's position.
//
// It resolves names and nothing more. MAST's archive searches (CAOM
// observations, the mission catalogs) are not implemented, and the provider
// does not claim a cone search: until #1088 it did, with one that could only
// fail.
package mast
