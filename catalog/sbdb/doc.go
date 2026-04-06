// Package sbdb provides a catalog.Provider implementation for the NASA/JPL Small-Body Database (SBDB).
//
// The sbdb package allows seamless resolution of minor planets, asteroids, and comets by
// interacting with the primary ss-api.jpl.nasa.gov endpoints. It automatically parses
// resulting CSV structures into catalog.Target instances, preserving standard designations,
// primary names, and SPK-IDs.
//
// Note that small bodies do not possess fixed ICRS coordinates like deep sky objects;
// therefore, spatial properties are inherently empty representing initial state vectors
// only, meant for routing into an ephemeris execution pipeline.
package sbdb
