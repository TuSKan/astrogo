package constants

// pi is used internally to define angular conversion constants without
// importing the math package, keeping this package dependency-free.
// Precision matches math.Pi (nearest float64 to the true value).
const pi = 3.141592653589793238462643383279502884197

// ── Physical constants ────────────────────────────────────────────────────────

// SpeedOfLight is the speed of light in vacuum, in metres per second.
// This value is exact by the 2019 SI redefinition of the metre.
// Source: CODATA 2018 / BIPM.
const SpeedOfLight = 299_792_458.0 // m/s

// ── Astronomical constants ────────────────────────────────────────────────────

// AstronomicalUnit is the length of one astronomical unit in metres.
// This is the IAU 2012 nominal value (B.2), fixed by resolution.
// Source: IAU 2012 Resolution B2.
const AstronomicalUnit = 1.495_978_707e11 // m

// JulianDaySeconds is the number of SI seconds in one Julian day.
// This value is exact by definition (86400 = 24 × 60 × 60).
const JulianDaySeconds = 86400.0 // s

// MeanEarthRadius is the IAU nominal mean radius of the Earth in metres.
// This is a volumetric mean and is not identical to the WGS84 semi-major axis.
// Source: IAU 2015 Resolution B3, Table 1.
const MeanEarthRadius = 6_371_000.0 // m

// ── Earth figure (WGS84) ──────────────────────────────────────────────────────

// WGS84SemiMajorAxis is the semi-major axis of the WGS84 reference ellipsoid
// in metres. This value is exact within the WGS84 standard.
// Source: NGA.STND.0036_1.0.0_WGS84 (2014).
const WGS84SemiMajorAxis = 6_378_137.0 // m

// WGS84InverseFlattening is the inverse flattening 1/f of the WGS84 ellipsoid.
// The flattening f = 1/WGS84InverseFlattening ≈ 1/298.257.
// Source: NGA.STND.0036_1.0.0_WGS84 (2014).
const WGS84InverseFlattening = 298.257_223_563

// WGS84Flattening is the flattening f = (a - b) / a of the WGS84 ellipsoid,
// derived from WGS84InverseFlattening.
const WGS84Flattening = 1.0 / WGS84InverseFlattening

// ── Angular conversion constants ──────────────────────────────────────────────

// RadiansPerDegree is the number of radians in one degree (π / 180).
const RadiansPerDegree = pi / 180

// DegreesPerRadian is the number of degrees in one radian (180 / π).
const DegreesPerRadian = 180 / pi

// ArcSecondsPerRadian is the number of arc-seconds in one radian (3600 × 180 / π).
const ArcSecondsPerRadian = 3600 * DegreesPerRadian
