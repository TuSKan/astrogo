# WCSLIB reference table for FITS WCS

Generates `../wcs_astropy.json`, which `fits/wcs_astropy_test.go` reads. The
table has 15 headers and 731 pixel/world pairs.

```bash
cd fits/testdata/wcsfixture
uv run generate.py
```

`uv.lock` pins the toolchain, so a regeneration a year from now resolves the
same versions. The document also records the `astropy`, WCSLIB, `numpy` and
`python` versions, plus the astrogo commit it was generated against.

## Why Python is here

The repository's bar for a Python generator, set by `coord/testdata/rvfixture`,
is that there is **no route to the reference from Go**. WCSLIB is Mark
Calabretta's C implementation of the FITS WCS papers, which he co-authored. No
Go binding to it exists, and Astropy's `astropy.wcs` is how it is usually
reached.

As with the radial-velocity table, the generator writes a checked-in document
and the test only reads it. Ordinary `go test ./...` needs no Python, no
network and no `uv`.

## The same cards on both sides

Each case stores its FITS cards verbatim. Astropy builds its WCS from them
through `astropy.io.fits.Header`, and the Go test builds a `fits.Header` from
the same cards. Nothing about a header is re-expressed in between, so a
disagreement is about the transform and never about two readings of one
header.

## Case selection

The cases cover what `fits.ExtractWCS` reads:

- **Every projection it supports.** These are TAN, SIN, ARC, STG and AIT. AIT
  appears three times, because its native reference point lies on the equator
  (θ₀ = 0). The default LONPOLE is therefore 0° for a reference north of the
  equator and 180° south of it, so there is one case of each, plus the origin.
- **The linear part, three ways:**
  - a PC rotation;
  - a CD matrix with skew and unequal scales;
  - a CD matrix for an ordinary rotated sky image, with RA increasing to the
    left (CD1_1 < 0, CD2_2 > 0), turned 30°.

  A quarter-turn CD matrix is also included, whose zero diagonal says nothing
  about which way an axis points.
- **Where projections go wrong:** a reference point half a degree from the
  north pole, one a degree from the south pole, and one just below RA 360°
  with points on both sides of the wrap.
- **Galactic axes (GLON/GLAT)**, because the reader accepts them and the
  projection must not care which sphere it is on.

## What it found

On its first run, the two CD cases with axes of opposite sign failed at every
off-center point. `ExtractWCS` split CD into CDELT and PC by columns, so
CDELTᵢ·PCᵢⱼ came out as CDᵢⱼ·CDELTᵢ/CDELTⱼ. For an ordinary sky image that
flips the cross terms, and a rotated image was read rotated the other way:
324″ out at 900 pixels (#524). The PC-rotated case passed throughout. So did
the quarter turn, whose two default signs happen to agree.
