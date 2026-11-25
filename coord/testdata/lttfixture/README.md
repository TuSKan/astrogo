# Astropy reference table for the barycentric light-travel delay

Generates `../ltt_astropy.json`, which `coord/barycentric_delay_fixture_test.go`
reads. 175 cases: rvfixture's 5 sites × 5 epochs × 7 target directions, chosen
there for what they exercise (see `../rvfixture/README.md`).

```bash
cd coord/testdata/lttfixture
uv run generate.py
```

`uv.lock` pins the toolchain. The document records the `astropy` and `pyerfa`
versions and the astrogo commit it was generated against. Python is here for
the reason rvfixture gives: there is no route to Astropy's answer from Go, and
ordinary `go test ./...` needs neither Python nor `uv`.

## The two delay columns

`delay_geometric_s` projects the observer's barycentric position, Earth's from
erfa's `epv00` plus the site's GCRS position, on the direction of the target,
over c. That is the model `coord.Context.BarycentricDelay` implements, and the
test compares astrogo against it.

`delay_published_s` is Astropy's own `Time.light_travel_time`, and it differs.
It does not add the site to Earth: it transforms the site point ITRS → GCRS →
ICRS through its coordinate frames, and the GCRS → ICRS step treats a point at
finite distance as an apparent place, removing stellar aberration from its
geocentric direction. That turns the site vector through about v/c:

| | |
| :--- | ---: |
| the site moved, worst case (78° N, 2026-01-04) | 639 m |
| predicted, \|site\| · v/c | 642 m |
| delay gap, measured maximum over 175 cases | 2.105 µs |
| bound, R⊕ · (v⊕ + v_rot) / c² | 2.18 µs |

The IAU's GCRS → BCRS position transformation has no first-order term, so the
geometric sum is the observer's barycentric position, as Eastman et al. (2010)
take it for BJD. The test asserts the gap stays under that bound and reaches
at least half of it, so a change in either model shows.

## The arrival column

`bjd_tdb_jd1 + bjd_tdb_jd2` is the observer's TDB plus the geometric delay, as
Astropy's two-part Julian Date: a single float64 Julian Date resolves only about
40 µs. The test runs `ArrivalAtObserver` on it and requires the case's UTC
back, so astrogo's inverse is checked against Astropy's forward conversion.

## Shared ancestry

Both sides reach Earth's barycentric position by SOFA's `epv00`, so this is not
an independent check of the Earth ephemeris, which `ephemeris.sofa.sun`
compares with DE440. It checks the projection, the site geodesy, the time
scales, the sign and the inverse.
