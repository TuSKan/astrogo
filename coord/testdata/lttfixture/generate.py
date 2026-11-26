"""Generate the Astropy reference table for astrogo's barycentric light-travel delay.

Writes ../ltt_astropy.json, which coord/barycentric_delay_fixture_test.go reads.

    uv run generate.py

What it records
---------------
For each (site, epoch, target) case, the Roemer delay: the observer's
barycentric position projected on the direction of the target, over the speed
of light, with no Shapiro delay and the source at infinity. Two values of it:

- delay_published_s is Astropy's Time.light_travel_time(kind="barycentric").
- delay_geometric_s is the same projection of the geometric sum, Earth's
  barycentric position (erfa's epv00) plus the site's GCRS position
  (EarthLocation.get_gcrs_posvel), which is the model
  coord.Context.BarycentricDelay implements.

They differ because light_travel_time does not form that sum: it carries the
site through Astropy's frames, ITRS to GCRS to ICRS, and the GCRS-to-ICRS step
treats a point at finite distance as an apparent place and removes stellar
aberration from its geocentric direction. That rotates the site vector by
about v/c, moving it up to |site| v/c = 640 m and the delay up to 2.1
microseconds. The IAU's GCRS-to-BCRS position transformation has no
first-order term, so the geometric sum is the observer's barycentric position.
The Go test compares astrogo with the geometric value and asserts the size of
the gap to the published one, so a change on either side shows.

It also records the barycentric arrival time the geometric delay produces,
BJD_TDB = t_TDB + delay, as Astropy's two-part Julian Date. A single float64
Julian Date resolves only about 40 microseconds, which is coarser than the
agreement being tested. The Go test runs ArrivalAtObserver on that value and
requires the original UTC back, so the inverse astrogo implements is checked
against Astropy's forward conversion, not against itself.

The cases are rvfixture's: the same sites, epochs and targets, chosen there for
what they exercise. Their README explains the choice.
"""

import json
import platform
import subprocess
import sys
from datetime import datetime, timezone

import astropy
import erfa
import numpy
from astropy import constants as const
from astropy import units as u
from astropy.coordinates import EarthLocation, SkyCoord, solar_system_ephemeris
from astropy.time import Time

OUT = "../ltt_astropy.json"

SITES = [
    ("Greenwich", 0.0, 51.4772, 45.0),
    ("Paranal", -70.40417, -24.62722, 2635.0),
    ("Mauna Kea", -155.47441, 19.8263, 4145.0),
    ("Equator (synthetic, 0N 0E)", 0.0, 0.0, 0.0),
    ("Polar (synthetic, 78N)", 0.0, 78.0, 0.0),
]

EPOCHS = [
    ("2026-01-04T00:00:00", "near perihelion, Earth fastest"),
    ("2026-03-20T12:00:00", "March equinox"),
    ("2026-07-06T00:00:00", "near aphelion, Earth slowest"),
    ("2026-09-22T18:00:00", "September equinox"),
    ("2026-11-15T06:00:00", "an ordinary date, no special geometry"),
]

TARGETS = [
    ("ecliptic plane, RA 0", 0.0, 0.0),
    ("ecliptic plane, RA 180", 180.0, 0.0),
    ("just below the RA wrap", 359.99, 0.0),
    ("north ecliptic pole", 270.0, 66.5607),
    ("north celestial pole", 0.0, 89.9),
    ("Sirius", 101.287155, -16.716116),
    ("Vega", 279.234735, 38.783689),
]


def git_describe():
    """The astrogo revision this table was generated against."""
    try:
        out = subprocess.run(
            ["git", "rev-parse", "HEAD"],
            capture_output=True, text=True, check=True, cwd="../../..",
        )
        return out.stdout.strip()
    except Exception:
        return "unknown"


def main():
    cases = []

    for site_name, lon, lat, height in SITES:
        location = EarthLocation.from_geodetic(
            lon=lon * u.deg, lat=lat * u.deg, height=height * u.m,
        )

        for iso, why_epoch in EPOCHS:
            obstime = Time(iso, scale="utc")

            # The geometric observer: Earth's barycentric position by epv00 at
            # the observer's TDB, plus the site's GCRS position.
            _, earth_pvb = erfa.epv00(obstime.tdb.jd1, obstime.tdb.jd2)
            observer = (
                earth_pvb[0] * u.au.to(u.m)
                + location.get_gcrs_posvel(obstime)[0].xyz.to_value(u.m)
            )

            for target_name, ra, dec in TARGETS:
                target = SkyCoord(ra=ra * u.deg, dec=dec * u.deg)

                published = obstime.light_travel_time(
                    target, kind="barycentric", location=location, ephemeris="builtin",
                )

                toward = numpy.array(erfa.s2c(numpy.deg2rad(ra), numpy.deg2rad(dec)))
                geometric = observer @ toward / const.c.to_value(u.m / u.s)

                bjd = obstime.tdb + geometric * u.s

                cases.append({
                    "name": f"{target_name} @ {site_name}, {iso[:10]}",
                    "site": {
                        "name": site_name,
                        "lon_deg": lon,
                        "lat_deg": lat,
                        "height_m": height,
                    },
                    "epoch_utc": iso,
                    "epoch_note": why_epoch,
                    "target": {"name": target_name, "ra_deg": ra, "dec_deg": dec},
                    # repr-precision floats, so the reference round-trips exactly.
                    "delay_published_s": float(published.to_value(u.s)),
                    "delay_geometric_s": float(geometric),
                    "bjd_tdb_jd1": float(bjd.jd1),
                    "bjd_tdb_jd2": float(bjd.jd2),
                })

    doc = {
        "schema_version": 1,
        "generated": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "astrogo_commit": git_describe(),
        "generator": {
            "astropy": astropy.__version__,
            "pyerfa": erfa.__version__,
            "numpy": numpy.__version__,
            "python": platform.python_version(),
        },
        "model": (
            "The Roemer delay: the observer's barycentric position dotted with the "
            "unit vector to the target, over c, with no Shapiro delay and the source "
            "at infinity. delay_geometric_s takes the position as Earth's (erfa's "
            "epv00 at the observer's TDB) plus the site's GCRS position, the model "
            "astrogo implements. delay_published_s is Astropy's "
            "Time.light_travel_time(kind='barycentric', ephemeris='builtin'), which "
            "carries the site through its frames and so removes stellar aberration "
            "from the site's geocentric direction, up to |site| v/c = 640 m, 2.1 us "
            "of delay. bjd_tdb_jd1 + bjd_tdb_jd2 is the observer's TDB plus the "
            "geometric delay, the barycentric arrival time."
        ),
        "shared_ancestry": (
            "Astropy reaches Earth's barycentric position through pyerfa's epv00 and "
            "astrogo through gofa's; both are SOFA's algorithm. The table validates "
            "the projection, the site geodesy, the time scales, the sign and the "
            "inverse, not the Earth ephemeris, which astrogo checks separately "
            "against DE440."
        ),
        "cases": cases,
    }

    with open(OUT, "w", encoding="utf-8", newline="\n") as f:
        json.dump(doc, f, indent=2)
        f.write("\n")

    print(f"wrote {len(cases)} cases to {OUT}", file=sys.stderr)
    print(f"  astropy {astropy.__version__}, pyerfa {erfa.__version__}", file=sys.stderr)


if __name__ == "__main__":
    with solar_system_ephemeris.set("builtin"):
        main()
