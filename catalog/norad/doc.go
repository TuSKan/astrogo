// Package norad provides a client for NORAD General Perturbations (GP) orbital
// data from CelestTrak. It fetches satellite element sets in the OMM-compatible
// JSON format defined by the CCSDS 502.0-B-3 standard (Orbit Mean-Elements
// Message), with field names aligned to the Space Data Standards project
// (https://spacedatastandards.org).
//
// # Data Source
//
// All queries use the CelestTrak GP API:
//
//	https://celestrak.org/NORAD/elements/gp.php?{QUERY}=VALUE&FORMAT=JSON
//
// Supported query types:
//   - [QueryCatNr]  — Catalog Number (1 to 9 digits)
//   - [QueryIntDes] — International Designator (yyyy-nnn)
//   - [QueryGroup]  — CelestTrak satellite groups (STATIONS, STARLINK, etc.)
//   - [QueryName]   — Satellite name search (partial match)
//   - [QuerySpecial] — Special datasets (GPZ, GPZ-PLUS, DECAYING)
//
// # Rate Limiting
//
// CelestTrak updates GP data approximately once every 2 hours and enforces
// strict rate limits. The client caches results locally via [resolve.Cache]
// and respects a minimum 2-hour polling interval. Excessive requests will
// result in IP blocking. See https://celestrak.org/NORAD/documentation/gp-data-formats.php
// for the full usage policy.
//
// # Architecture
//
// The package implements [resolve.Provider] for integration with the unified
// catalog [Resolver], and provides a lower-level [Provider.Fetch] method for
// direct access to parsed GP element sets.
package norad
