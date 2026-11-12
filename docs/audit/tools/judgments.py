"""The audit map's judgments: what the facts collected by collect.py cannot say.

Everything here is a reading of the code or a record of what an audit run did,
written by hand. Edit it when an audit step changes one of them: a sweep that
finds nothing adds its packages to SWEPT_CLEAN, a "seen, not filed" item leaves
NOTED once it is filed or judged, and a finished sweep is appended to SWEEPS.
"""

# Packages that decode bytes from a server or a file, read from each package's
# entry points. Each should carry a fuzz target of its own.
EXTERNAL = {
    "catalog/fink", "catalog/gaia", "catalog/jpl", "catalog/mast", "catalog/mpcorb", "catalog/norad",
    "catalog/openngc", "catalog/sbdb", "catalog/simbad", "catalog/vizier",
    "ephemeris/jpl/lsk", "ephemeris/jpl/spk", "ephemeris/satellite", "ephemeris/satellite/sgp4",
    "fits", "internal/votable", "time/internal/iers", "plan", "atmosphere/dataset/cams",
    "skybrightness/dataset/airglow", "skybrightness/dataset/crosssection", "skybrightness/dataset/dust",
    "skybrightness/dataset/luminosity", "skybrightness/dataset/passband", "skybrightness/dataset/raster",
    "skybrightness/dataset/solar", "skybrightness/dataset/starlight", "skybrightness/dataset/viirs",
}

# Packages whose API takes floating-point inputs where non-finite values matter.
NUMERIC = {
    "angle", "atmosphere", "coord", "constellation", "ephemeris", "ephemeris/core", "ephemeris/kepler",
    "ephemeris/jpl", "ephemeris/satellite", "ephemeris/satellite/sgp4", "magnitude", "optics", "plan",
    "skybrightness", "time", "unit", "vector", "catalog/simbad", "catalog/vizier", "catalog/gaia",
}

# Checked for non-finite inputs and found already guarded.
SWEPT_CLEAN = {"optics", "unit", "catalog/openngc", "coord", "vector", "ephemeris/core", "ephemeris/jpl"}

# Audited in depth against an outside reference, beyond the findings already
# counted, so a low finding density is not a light audit. Each names the check
# that cleared it.
AUDITED = {
    "constellation": "Roman's table over 250,000 points, boundaries, poles and the 0h/24h wrap; astropy compared",
}

# Seen during an audit run and judged, but not filed.
NOTED = {
    "plan": "Slew time converts a float to time.Duration unguarded; overflow needs a rate below 2e-8 deg/s "
            "(judged unreachable).",
    "vector": "A magnitude beyond about 1e154 overflows to infinity when squared (judged unreachable: no "
              "astronomical vector here is near it).",
}

# The systematic checks of each audit run: one class of defect, across a scope.
SWEEPS = [
    {"date": "2026-11-10", "sweep": "Non-finite values in third-party parsers",
     "scope": "skybrightness/dataset/*, time/internal/iers, catalog parsers",
     "outcome": "4 issues fixed; non-finite coordinates in catalog parsers seen and not filed",
     "issues": "#1030 #1032 #1033 #1037"},
    {"date": "2026-11-10", "sweep": "Range checks that let infinity through",
     "scope": "whole module (grep, then a probe per site)",
     "outcome": "8 functions fixed; 2 rejected as already refusing the result", "issues": "#1038"},
    {"date": "2026-11-10", "sweep": "Cached copies a provider's parser refuses",
     "scope": "all 12 remote.GetFile callers",
     "outcome": "2 providers fixed; MPCORB, OpenNGC and the rest already heal", "issues": "#1015 #1043"},
    {"date": "2026-11-10", "sweep": "Parsers that accept an error page as an empty file",
     "scope": "IERS, MPC codes, LSK, OpenNGC, luminosity, cross sections, solar, airglow",
     "outcome": "1 parser fixed (IERS); the others refuse it", "issues": "#1042"},
    {"date": "2026-11-11", "sweep": "Refused updates deleting the cached copy",
     "scope": "remote.WithRead, every mutable endpoint",
     "outcome": "Fixed in remote for every provider", "issues": "#1047"},
    {"date": "2026-11-10", "sweep": "Fuzz targets whose queries come from parsed data",
     "scope": "all 24 fuzz packages",
     "outcome": "1 blind target fixed (IERS); the others query fixed inputs", "issues": "#1037"},
    {"date": "2026-11-10", "sweep": "Tests whose comments admit a defect", "scope": "every _test.go",
     "outcome": "2 tests rewritten to test the fix", "issues": "#1042 #1044"},
    {"date": "2026-11-10", "sweep": "Float to integer and Duration conversions", "scope": "whole module",
     "outcome": "1 site, judged unreachable; not filed", "issues": ""},
    {"date": "2026-11-10", "sweep": "Upstream service drift", "scope": "Horizons, SIMBAD",
     "outcome": "SIMBAD outage now skips; Horizons CSV recovered; Horizons TDB−UT freeze reported to JPL, "
                "which fixed it on 11 November",
     "issues": "#1023 #1026 #1045"},
    {"date": "2026-11-11", "sweep": "SGP4 against an independent implementation",
     "scope": "ephemeris/satellite/sgp4: python-sgp4's C++ over 5,000 synthetic element sets, and non-finite fields",
     "outcome": "Propagation agrees; an At hang on resonant orbits and doc drift fixed", "issues": "#1052 #1054"},
    {"date": "2026-11-11", "sweep": "Constellation boundaries against Roman's table",
     "scope": "constellation: 250,000 points, boundaries, poles and the 0h/24h wrap; astropy compared",
     "outcome": "2 points disagree, each within 0.2″ of a boundary; a NaN position fixed", "issues": "#1055"},
    {"date": "2026-11-11", "sweep": "Non-finite inputs, remaining numeric packages",
     "scope": "vector, ephemeris/core, ephemeris/jpl, ephemeris/satellite, every catalog parser",
     "outcome": "3 packages clean; satellite fixed; catalog parsers read a non-finite number as absent",
     "issues": "#1052 #1080"},
    {"date": "2026-11-11", "sweep": "Fuzz targets for the remaining parsers of outside data",
     "scope": "catalog/fink, catalog/mast, catalog/sbdb, skybrightness/dataset/viirs",
     "outcome": "3 defects fixed, 2 of them panics; xmatch reads no bytes and needs none",
     "issues": "#1058 #1061 #1062"},
    {"date": "2026-11-11", "sweep": "Coverage of six packages to 75%",
     "scope": "skybrightness/dataset, starlight, airglow, logging, catalog/internal/posmatch, catalog/mast",
     "outcome": "All six at 77% or more; 10 defects found, 9 fixed, 1 awaiting a decision",
     "issues": "#1066 #1067 #1068 #1069 #1077 #1078 #1079 #1088 #1089 #1090"},
]
