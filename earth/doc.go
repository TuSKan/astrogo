// Package earth provides Earth geodesy and Earth-fixed coordinate primitives.
//
// # Design
//
// The package focuses on terrestrial geometry:
//   - [Ellipsoid]: Defines the Earth's shape (e.g., WGS84).
//   - [Geodetic]: Represents points on the Earth's surface (Lon, Lat, Height).
//
// # Coordinate Systems
//
// This package handles transformations between:
//   - Geodetic: (longitude, latitude, height above ellipsoid).
//   - ECEF (Earth-Centered, Earth-Fixed): Cartesian (X, Y, Z) vectors in
//     the Earth's rotating frame.
//
// # Scope
//
// This package owns physical Earth modeling. It does NOT own:
//   - Observatory/Site modeling (see package `observatory`).
//   - Sidereal time or atmospheric refraction (see package `refraction`).
//   - Ephemerides for Earth's center of mass (see package `body`).
package earth
