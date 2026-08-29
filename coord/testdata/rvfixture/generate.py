"""Generate the Astropy reference table for astrogo's radial-velocity correction.

Writes ../rv_astropy.json, which coord/radialvelocity_fixture_test.go reads.

    uv run generate.py

Why this exists
---------------
astrogo's BarycentricRVCorrection and HeliocentricRVCorrection are public API
validated by invariants alone: an annual sinusoid of the right amplitude, a zero
for a perpendicular target, a sign flip for antipodal ones, a diurnal amplitude
scaling as cos(latitude). Those are strong, and they share a blind spot — an
identity cannot catch an error that both of its sides make.

Every other reference astrogo validates against has a Go client already (JPL
Horizons, USNO, the NASA canons, VizieR, ESO SkyCalc). This one does not, and
that is the whole justification for a Python generator living in this
repository: there is no route to Astropy's answer from Go.

Sign convention
---------------
Astropy's SkyCoord.radial_velocity_correction returns the velocity to ADD to a
measured topocentric radial velocity to refer it to the requested frame, which
is the same convention astrogo documents:

    rv_barycentric = rv_measured + correction

If that ever diverges the test will show a clean sign flip across every case
rather than scatter, which is a diagnosis rather than a puzzle.
"""

import json
import platform
import subprocess
import sys
from datetime import datetime, timezone

import astropy
import erfa
import numpy
from astropy import units as u
from astropy.coordinates import (
    EarthLocation,
    SkyCoord,
    UnitSphericalRepresentation,
    get_body_barycentric_posvel,
)
from astropy.time import Time

OUT = "../rv_astropy.json"

# Sites, named and chosen for what they exercise rather than for being
# observatories. The diurnal term scales as cos(latitude), so the equator and
# the polar site bracket it; altitude enters through the geocentric radius, so
# one high site is included; longitude decides the phase of the diurnal term,
# so they are spread.
SITES = [
    ("Greenwich", 0.0, 51.4772, 45.0),
    ("Paranal", -70.40417, -24.62722, 2635.0),
    ("Mauna Kea", -155.47441, 19.8263, 4145.0),
    ("Equator (synthetic, 0N 0E)", 0.0, 0.0, 0.0),
    ("Polar (synthetic, 78N)", 0.0, 78.0, 0.0),
]

# Epochs across one year plus the two that bracket Earth's orbital speed.
# Perihelion is early January and aphelion early July, so those two carry the
# largest and smallest annual amplitude the correction can have.
EPOCHS = [
    ("2026-01-04T00:00:00", "near perihelion, Earth fastest"),
    ("2026-03-20T12:00:00", "March equinox"),
    ("2026-07-06T00:00:00", "near aphelion, Earth slowest"),
    ("2026-09-22T18:00:00", "September equinox"),
    ("2026-11-15T06:00:00", "an ordinary date, no special geometry"),
]

# Target directions, chosen to span the projection rather than the sky at
# random. The ecliptic plane sees the full annual term, the ecliptic pole sees
# almost none of it, and the RA-wrap case is there because a longitude
# discontinuity is where a projection bug hides.
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
            # No location on the Time: Astropy refuses the call outright if
            # the observer is specified twice, and passing it to the method is
            # the form that reads as "this correction, for this observer".
            obstime = Time(iso, scale="utc")

            for target_name, ra, dec in TARGETS:
                target = SkyCoord(ra=ra * u.deg, dec=dec * u.deg)

                bary = target.radial_velocity_correction(
                    kind="barycentric", obstime=obstime, location=location,
                )
                helio = target.radial_velocity_correction(
                    kind="heliocentric", obstime=obstime, location=location,
                )

                # Astropy's barycentric value is not a plain projection: it
                # combines the observer's velocity with the line of sight
                # relativistically and adds gravitational redshift, which
                # together sit about 4.65 m/s above the classical answer
                # (second-order Doppler v^2/2c = 1.48, solar redshift
                # GM/rc = 2.96, Earth's own = 0.21). Its heliocentric value,
                # by contrast, is a plain dot product — which is why the two
                # agree with astrogo to wildly different precision.
                #
                # astrogo is explicitly classical and says so. Comparing it
                # against the relativistic value would measure that documented
                # difference and call it an error, so the classical projection
                # is reconstructed here from Astropy's own pieces: the same
                # ephemeris, the same observer velocity, the same model. This
                # is what Astropy itself computes before the relativistic
                # combination, and what its heliocentric branch returns.
                observer_v = (
                    get_body_barycentric_posvel("earth", obstime)[1]
                    + location.get_gcrs_posvel(obstime)[1]
                )
                targcart = target.icrs.represent_as(
                    UnitSphericalRepresentation
                ).to_cartesian()
                bary_classical = observer_v.dot(targcart).to(u.km / u.s)

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
                    # repr-precision floats: the values must round-trip
                    # exactly, since truncating a reference for readability
                    # makes it a different reference.
                    "barycentric_km_s": float(bary.to(u.km / u.s).value),
                    "barycentric_classical_km_s": float(bary_classical.value),
                    "heliocentric_km_s": float(helio.to(u.km / u.s).value),
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
        "barycentric_note": (
            "barycentric_km_s is Astropy's full value, which is relativistic: it "
            "combines the observer velocity with the line of sight and adds "
            "gravitational redshift, about 4.65 m/s above a classical projection. "
            "barycentric_classical_km_s is the plain projection of the same observer "
            "velocity onto the same line of sight, which is the model astrogo "
            "implements and documents. Compare against the classical one; the gap "
            "between them is a stated model difference, not an error in either."
        ),
        "convention": (
            "the velocity to ADD to a measured topocentric radial velocity to "
            "refer it to the named frame, in km/s; matches astrogo's "
            "rv_barycentric = rv_measured + correction"
        ),
        "shared_ancestry": (
            "Astropy computes the observer's barycentric velocity through "
            "pyerfa, and astrogo through gofa; both are SOFA-derived, and both "
            "reach it by the same epv00 algorithm. This table therefore "
            "validates the projection, the site geodesy, the time scales and "
            "the sign convention, and cannot validate the underlying Earth "
            "ephemeris — which astrogo checks separately against DE440."
        ),
        "cases": cases,
    }

    # newline="\n" so the document is byte-identical whichever
    # platform generated it. Python translates "\n" to the host line
    # ending by default, which on Windows rewrites all 4,000 lines on every
    # regeneration and leaves a reviewer unable to see which numbers actually
    # moved — the diff being reviewable is the whole point of checking the
    # table in.
    with open(OUT, "w", encoding="utf-8", newline="\n") as f:
        json.dump(doc, f, indent=2)
        f.write("\n")

    print(f"wrote {len(cases)} cases to {OUT}", file=sys.stderr)
    print(f"  astropy {astropy.__version__}, pyerfa {erfa.__version__}", file=sys.stderr)


if __name__ == "__main__":
    main()
