# Astropy reference table for radial-velocity correction

Generates `../rv_astropy.json`, which `coord/radialvelocity_fixture_test.go`
reads. 175 cases: 5 named sites × 5 epochs × 7 target directions.

```bash
cd coord/testdata/rvfixture
uv run generate.py
```

`uv.lock` pins the toolchain, so a regeneration a year from now resolves the
same versions. The document records `astropy`, `pyerfa`, `numpy` and `python`
versions plus the astrogo commit it was generated against, so a figure taken
from it stays checkable after both sides have moved on.

## Why Python is here at all

This is the only Python in the repository, and the bar for adding it was that
there is **no route to this reference from Go**. Every other reference astrogo
validates against has a Go client already — JPL Horizons, USNO, the NASA
eclipse canons, VizieR, ESO SkyCalc, IRSA. Astropy does not, and
`SkyCoord.radial_velocity_correction` is the only widely-used independent
implementation of this particular calculation.

The generator writes a checked-in JSON document rather than being run by the
test. Ordinary `go test ./...` needs no Python, no network and no `uv`.

## The two barycentric columns

`barycentric_km_s` is Astropy's own answer and it is **relativistic**: it
combines the observer's velocity with the line of sight relativistically and
adds gravitational redshift. `barycentric_classical_km_s` is the plain
projection of the same observer velocity onto the same line of sight.

astrogo implements the classical model and documents that it does, so the test
compares against the classical column. Comparing against the relativistic one
measures a difference both projects already describe in their own
documentation, and calling that an error would be measuring the wrong thing.

The gap between them is **not** ignored — the test asserts its size. Measured
at 4.66 m/s, against 4.65 predicted from the terms astrogo names as omitted:

| term | m/s |
| :--- | ---: |
| second-order Doppler, v²/2c | 1.481 |
| solar gravitational redshift, GM☉/rc | 2.959 |
| Earth's own gravitational redshift, GM⊕/Rc | 0.209 |
| **total** | **4.649** |

If astrogo ever acquires a defect the like-for-like comparison cannot see, or
Astropy changes its model, that number moves and the test says so.

## Shared ancestry

Astropy reaches the observer's barycentric velocity through `pyerfa` and
astrogo through `gofa`. Both are SOFA-derived and both arrive by the same
`epv00` algorithm, so **this is not an independent check of the Earth
ephemeris**. It checks what astrogo puts around that shared core: the
projection, the sign convention, the site geodesy and the time scales.

The shared part is covered separately — `ephemeris.sofa.sun` compares the same
`epv00` against DE440. The two together do cover it; neither does alone.

## Case selection

Sites bracket what the correction depends on rather than listing observatories:
the equator and a 78°N site bound the diurnal term, which scales as cos φ;
Mauna Kea at 4145 m exercises the geocentric radius; longitudes are spread so
the diurnal phase differs.

Epochs include perihelion and aphelion, where Earth's orbital speed — and so
the annual term — is largest and smallest.

Targets span the projection rather than the sky: the ecliptic plane sees the
full annual term, the ecliptic pole almost none of it, and one target sits just
below the RA wrap, because a longitude discontinuity is where a projection bug
hides.
