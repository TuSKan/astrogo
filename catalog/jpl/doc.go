// Package jpl provides a catalog.Provider implementation handling metadata resolution
// through the NASA JPL Horizons system.
//
// Horizons maintains ephemerides and physical metadata for planets, natural satellites,
// major barycenters, and artificial spacecraft. The jpl catalog searches the horizons.api 
// endpoint strictly to fetch base identifiers and body names, allowing deep cross-matching 
// of objects against active spacecraft ID numbers.
//
// This package is exclusively for resolving static object identities. For resolving
// actual time-dependent position states and astrometric geometries, utilize the
// ephemeris/jpl library instead.
package jpl
