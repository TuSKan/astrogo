"""The audit map's judgments: what the facts collected by collect.py cannot say.

Everything here is a reading of the code or a record of what an audit run did,
written by hand. Edit it when an audit step changes one of them: a sweep that
finds nothing adds its packages to SWEPT_CLEAN, a "seen, not filed" item leaves
NOTED once it is filed or judged, and a finished sweep is appended to SWEEPS. A
closed bug the map cannot attribute goes into FIXED_BY when a commit fixed it
without naming it, and into OUTSIDE_PACKAGES when no package's code did.
"""

# Packages that decode bytes from a server or a file, read from each package's
# entry points. Each should carry a fuzz target of its own.
EXTERNAL = {
    "catalog/fink", "catalog/gaia", "catalog/horizons", "catalog/mast", "catalog/mpcorb", "catalog/norad",
    "catalog/openngc", "catalog/sbdb", "catalog/simbad", "catalog/vizier",
    "ephemeris/jpl/lsk", "ephemeris/jpl/spk", "ephemeris/satellite", "ephemeris/satellite/sgp4",
    "fits", "internal/votable", "time/internal/iers", "plan", "atmosphere/dataset/cams",
    "skybrightness/dataset/airglow", "skybrightness/dataset/crosssection", "skybrightness/dataset/dust",
    "skybrightness/dataset/luminosity", "skybrightness/dataset/passband", "skybrightness/dataset/raster",
    "skybrightness/dataset/solar", "skybrightness/dataset/starlight", "skybrightness/dataset/viirs",
}

# Packages that have moved, old path to new (#115). A commit that fixed a
# finding names the files as they were then, so collect.py reads each path
# through this to count the finding for the package as it is now.
RENAMED = {
    "catalog/fits": "catalog/fitscat",
    "catalog/jpl": "catalog/horizons",
    "fits/plan": "fits/fitsplan",
    "internal/horizons": "internal/horizonsfault",
    "skybrightness/plan": "skybrightness/skyplan",
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
# counted, so neither a low finding density nor the absence of any finding is a
# gap. Each names the check that cleared it.
AUDITED = {
    "constellation": "Roman's table over 250,000 points, boundaries, poles and the 0h/24h wrap; astropy compared",
    "internal/refraction": "the transcribed SOFA series against gofa's Atioq and Atoiq above the hand-over, "
                           "to 1.4e-4 µas (#1105)",
    "internal/oracle": "the 6,656 exact Kepler roots of the contract sweep against mpmath at 120 digits, "
                       "to 2⁻²¹⁴ relative",
}

# Closed bugs whose fixing commit never named them, each with the commit that
# fixed it, read from the pull request that closed the issue. collect.py
# attributes each to the packages that commit changed.
FIXED_BY = {
    694: "e17454fc",  # #699's body closes #694; the commit names only #696, the ΔT defect behind both
}

# Closed bugs no package's Go code fixed, each with where the fix was instead.
# They are not findings about a package, so the map counts them in none.
OUTSIDE_PACKAGES = {
    109: "the module's go directive, raised to 1.27 in go.mod by #231",
    140: "a behavior of go test -fuzz, documented in CLAUDE.md's fuzzing section",
    660: "CI: the Validation workflow's IRSA dust cache (#662)",
    680: "a dependency advisory, fixed in go.mod (#682)",
    685: "validation fixture data, after Horizons changed its answers (#687)",
    885: "the examples module (#887)",
    923: "CI: CodeQL as advanced setup (#924)",
    942: "CI: the changelog check (#949)",
    943: "Dependabot's configuration",
    1019: "CI: the changelog check (#1020)",
    1021: "a dependency advisory, fixed in go.mod (#1022)",
    1023: "upstream: JPL Horizons served the vectors again",
    1045: "upstream: fixed at JPL Horizons",
    1057: "a tracking issue; its four pull requests' findings are attributed",
    1092: "CI: the race job's report (#1096)",
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
    {"date": "2026-11-12", "sweep": "Test infrastructure, each bound canaried with its fault",
     "scope": "internal/leakcheck, internal/metrology, internal/refraction against gofa, internal/oracle against "
              "mpmath",
     "outcome": "4 defects fixed, one of them (a resty buffer race) found along the way; 1 awaiting a decision; "
                "refraction and oracle clean",
     "issues": "#1099 #1100 #1104 #1107 #1109"},
]
