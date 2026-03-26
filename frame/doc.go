// Package frame defines coordinate system identities and metadata.
//
// # Design
//
// A Frame represents a reference system (e.g., ICRS, AltAz) and carries the
// metadata required to define that system uniquely. For example:
//   - An [Ecliptic] frame may be defined by an equinox epoch.
//   - An [AltAz] frame is defined by an observer's location and time.
//
// # Lightweight Abstraction
//
// Frames are designed as lightweight value types or markers. They do not
// contain the mathematical logic for transformations; that logic lives in the
// `transform` package, which uses these frame definitions to select the
// correct algorithms.
package frame
