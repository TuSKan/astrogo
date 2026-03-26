# JPL Horizons Validation Fixtures

These fixtures provide "golden" state vectors for validating the `astrogo/ephemeris/jpl` provider.

## Settings

- **Reference Frame**: ICRF / J2000.0
- **Coordinate Center**: Earth (Geocentric) [500]
- **Time Specification**: TDB (Barycentric Dynamical Time)
- **Output Units**: AU and AU/day
- **Ephemeris Version**: DE440/441 (JPL Current Default)

## Data Points

Vectors were extracted for the following epochs:
1. `2000-01-01 12:00:00 TDB` (JD 2451545.0)
2. `2024-01-01 00:00:00 TDB` (JD 2460310.5)

## Discrepancies

Minor differences (sub-meter) are expected due to:
- Numerical interpolation differences between the BSP kernel and the Horizons online system.
- Tiny differences between DE441 (Horizons) and DE442 (Local BSP).
