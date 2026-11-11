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
    "catalog/openngc", "catalog/sbdb", "catalog/simbad", "catalog/vizier", "catalog/xmatch",
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
SWEPT_CLEAN = {"optics", "unit", "catalog/openngc", "coord"}

# Seen during an audit run and judged, but not filed.
NOTED = {
    "catalog/simbad": "Coordinates and magnitudes parsed as non-finite values are kept in targets (seen, not filed).",
    "catalog/vizier": "Coordinates and epochs parsed as non-finite values are kept in targets (seen, not filed).",
    "catalog/gaia": "Optional fields parsed as non-finite values are kept; VOTable may encode nulls that way "
                    "(seen, not filed).",
    "plan": "Slew time converts a float to time.Duration unguarded; overflow needs a rate below 2e-8 deg/s "
            "(judged unreachable).",
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
]
