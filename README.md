# Astrogo 🌌
High-Precision Astronomical Planning and Celestial Mechanics in Pure Go

Astrogo is an end-to-end, statically typed observation planner built for astrophotography and observatory scheduling. Designed as a high-performance alternative to Python's astroplan, it provides rigorous coordinate transformations, offline deep-sky target resolution, and dynamic constraint solving without the overhead of CGO.

Whether you are calculating the precise transit time of a distant nebula or verifying that a target fits perfectly within the field of view of a full-frame sensor paired with a 70-200mm lens and a 2x teleconverter, Astrogo delivers sub-arcsecond accuracy entirely natively in Go.

## Core Capabilities:

- **Pure Go Architecture**: Frictionless cross-compilation with zero CGO dependencies.
- **Rigorous Celestial Math**: Wraps IAU SOFA algorithms for highly accurate coordinate transformations (ICRS to local Topocentric Alt/Az) and time scale conversions (UTC, UT1, TT, LST).
- **Native JPL Ephemerides**: Direct binary parsing of NASA JPL DE440/DE441 files for pinpoint solar system object tracking.
- **Zero-Latency Target Lookups**: Embedded OpenNGC catalog for instant, offline deep-sky object resolution (e.g., "M42", "NGC 224").
- **Automated IERS Pipelines**: Standalone module handling Earth Orientation Parameters to account for unpredictable planetary rotation.
- **Constraint-Based Scheduling**: Fast evaluation engines to filter observing windows by altitude, airmass, moon separation, and local twilight.
- **Optical Framing Engine**: Hardware-aware modules to calculate exact image scale and Field of View (FOV) based on specific camera sensors and lens configurations.