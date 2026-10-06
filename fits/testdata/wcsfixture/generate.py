"""Generate the Astropy/WCSLIB reference table for astrogo's FITS WCS.

Writes ../wcs_astropy.json, which fits/wcs_astropy_test.go reads.

    uv run generate.py

Why this exists
---------------
astrogo's WCS.PixelToWorld and WorldToPixel were tested only against
themselves: the reference pixel maps to CRVAL, a pixel round-trips, a step
along an axis moves the sky the way CDELT's sign says. A projection wrong the
same way in both directions passes all of that (#523).

WCSLIB is the reference implementation of Calabretta & Greisen, and it is C.
There is no route to it from Go, and Astropy is how it is usually reached,
which is the bar the repository sets for a Python generator.

Each case records its FITS cards verbatim, and both sides read those same
cards: Astropy through astropy.io.fits.Header, astrogo through fits.Header.
Nothing about the header is re-expressed in between, so a disagreement is a
disagreement about the transform.
"""

import json
import platform
import subprocess
from datetime import datetime, timezone

import astropy
import numpy
from astropy.io import fits
import astropy.wcs._wcs
from astropy.wcs import WCS


OUT = "../wcs_astropy.json"


def header(ctype, crval, crpix=(100.0, 200.0), cdelt=None, pc=None, cd=None):
    """The cards of one celestial header, as the strings a FITS file holds."""
    cards = {
        "NAXIS": 2,
        "CTYPE1": ctype[0],
        "CTYPE2": ctype[1],
        "CRVAL1": crval[0],
        "CRVAL2": crval[1],
        "CRPIX1": crpix[0],
        "CRPIX2": crpix[1],
    }
    if cdelt is not None:
        cards["CDELT1"], cards["CDELT2"] = cdelt
    if pc is not None:
        (cards["PC1_1"], cards["PC1_2"]), (cards["PC2_1"], cards["PC2_2"]) = pc
    if cd is not None:
        (cards["CD1_1"], cards["CD1_2"]), (cards["CD2_1"], cards["CD2_2"]) = cd
    return cards


def offsets(span, step):
    """Pixel offsets from the reference pixel on a square grid, the
    reference itself included."""
    values = [v * step for v in range(-span, span + 1)]
    return [(dx, dy) for dx in values for dy in values]


ROT30 = ((0.8660254037844387, -0.5), (0.5, 0.8660254037844387))

# Each case exercises something specific; the name says what.
CASES = [
    ("TAN, northern reference", header(("RA---TAN", "DEC--TAN"), (10.0, 20.0), cdelt=(-0.1, 0.1)), offsets(3, 100)),
    ("SIN, southern reference", header(("RA---SIN", "DEC--SIN"), (200.0, -30.0), cdelt=(-0.1, 0.1)), offsets(3, 100)),
    ("ARC, equatorial reference", header(("RA---ARC", "DEC--ARC"), (45.0, 0.0), cdelt=(-0.1, 0.1)), offsets(3, 150)),
    ("STG, northern reference", header(("RA---STG", "DEC--STG"), (10.0, 20.0), cdelt=(-0.1, 0.1)), offsets(3, 150)),
    # AIT's native reference point is on the equator (theta0 = 0), so the
    # default LONPOLE is 0 for a reference north of it and 180 south of it:
    # one of each, plus the origin.
    ("AIT, all-sky at the origin", header(("RA---AIT", "DEC--AIT"), (0.0, 0.0), cdelt=(-1.0, 1.0)), offsets(3, 25)),
    ("AIT, northern reference", header(("RA---AIT", "DEC--AIT"), (10.0, 20.0), cdelt=(-1.0, 1.0)), offsets(3, 20)),
    ("AIT, southern reference", header(("RA---AIT", "DEC--AIT"), (200.0, -30.0), cdelt=(-1.0, 1.0)), offsets(3, 20)),
    # The linear part: a rotation through PC, and a CD matrix with skew,
    # which a reader decomposing CD into CDELT and PC has to get right.
    ("TAN, PC rotated 30 degrees", header(("RA---TAN", "DEC--TAN"), (150.0, 2.0), cdelt=(-0.05, 0.05), pc=ROT30), offsets(3, 100)),
    ("TAN, CD with skew", header(("RA---TAN", "DEC--TAN"), (83.8, -5.4),
                                 cd=((-1.0e-3, 2.0e-4), (1.5e-4, 1.1e-3))), offsets(3, 300)),
    # The CD matrix an ordinary rotated sky image carries: right ascension
    # increasing to the left (CD1_1 < 0), declination up (CD2_2 > 0), turned
    # 30 degrees. HST, JWST and the survey pipelines write exactly this.
    ("TAN, CD rotated 30 degrees with sky parity", header(("RA---TAN", "DEC--TAN"), (150.0, 2.0),
                                                          cd=((-0.8660254037844387e-4, 0.5e-4),
                                                              (0.5e-4, 0.8660254037844387e-4))), offsets(3, 300)),
    # A quarter turn leaves the diagonal zero, so nothing there says which
    # way an axis points.
    ("TAN, CD rotated 90 degrees", header(("RA---TAN", "DEC--TAN"), (150.0, 2.0),
                                          cd=((0.0, 1.0e-4), (1.0e-4, 0.0))), offsets(3, 300)),
    # Where projections go wrong: next to the pole, and across RA 0/360.
    ("TAN, reference 0.5 degrees from the north pole", header(("RA---TAN", "DEC--TAN"), (60.0, 89.5), cdelt=(-0.05, 0.05)), offsets(3, 40)),
    ("SIN, reference 1 degree from the south pole", header(("RA---SIN", "DEC--SIN"), (300.0, -89.0), cdelt=(-0.05, 0.05)), offsets(3, 60)),
    ("TAN, reference just below RA 360", header(("RA---TAN", "DEC--TAN"), (359.8, 10.0), cdelt=(-0.1, 0.1)), offsets(3, 10)),
    # A non-equatorial axis pair: the reader accepts GLON/GLAT and the
    # projection must not care which sphere it is on.
    ("ARC, galactic axes", header(("GLON-ARC", "GLAT-ARC"), (120.0, -10.0), cdelt=(-0.2, 0.2)), offsets(3, 50)),
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

    for name, cards, grid in CASES:
        h = fits.Header()
        for k, v in cards.items():
            h[k] = v

        w = WCS(h)

        points = []
        for dx, dy in grid:
            px, py = cards["CRPIX1"] + dx, cards["CRPIX2"] + dy

            # FITS pixels are 1-based, so origin=1 throughout: CRPIX means
            # the same pixel to both sides.
            lon, lat = w.wcs_pix2world([[px, py]], 1)[0]
            if not (numpy.isfinite(lon) and numpy.isfinite(lat)):
                continue

            points.append({"pixel": [px, py], "world": [float(lon), float(lat)]})

        cases.append({
            "name": name,
            "header": cards,
            "lonpole": float(w.wcs.lonpole),
            "latpole": float(w.wcs.latpole),
            "points": points,
        })

    doc = {
        "schema_version": 1,
        "generated": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "astrogo_commit": git_describe(),
        "generator": {
            "astropy": astropy.__version__,
            "wcslib": astropy.wcs._wcs.WCSLIB_VERSION,
            "numpy": numpy.__version__,
            "python": platform.python_version(),
        },
        "convention": "pixel is FITS 1-based (origin=1); world is (longitude, latitude) in degrees",
        "cases": cases,
    }

    with open(OUT, "w", encoding="utf-8", newline="\n") as f:
        json.dump(doc, f, indent=1)
        f.write("\n")

    n = sum(len(c["points"]) for c in cases)
    print(f"wrote {OUT}: {len(cases)} headers, {n} points")


if __name__ == "__main__":
    main()
