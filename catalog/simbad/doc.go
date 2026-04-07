// Package simbad provides a provider.Provider implementation for the CDS SIMBAD Astronomical Database.
//
// The simbad package utilizes the Table Access Protocol (TAP) via the Astronomical Data
// Query Language (ADQL) to resolve object identifiers and fetch fundamental metadata
// such as Object Type (Otype), ICRS coordinates (RA/Dec), and standard catalog aliases.
// All requests are routed through a resilient, retry-aware provider.Client with native
// async streaming via SeqIterator.
//
// Cross-matching is explicitly handled by joining the central `basic` table against
// the `ident` properties natively returned as populated astrometric Target structures.
package simbad
