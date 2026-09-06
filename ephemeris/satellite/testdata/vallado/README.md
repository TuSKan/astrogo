# Vallado SGP4 verification data

Reference input and output for the SGP4/SDP4 propagator, from the test suite
published with:

> Vallado, D. A., Crawford, P., Hujsak, R., Kelso, T. S. (2006),
> "Revisiting Spacetrack Report #3", AIAA 2006-6753.

This is the suite every SGP4 implementation checks itself against, and the
reason astrogo can say anything at all about whether its propagator is
correct rather than merely plausible.

| file | what it is |
| --- | --- |
| `SGP4-VER.TLE` | 33 element sets. Each line 2 carries three extra trailing fields — start, stop and step in minutes since epoch — beyond the standard 69 columns. Lines beginning `#` are Vallado's own labels and say what each case is *for*. |
| `tcppver.out` | The reference states. One block per case, headed `<satnum> xx`, then one row per step: `tsince x y z vx vy vz` (minutes, km, km/s, TEME) followed by orbital elements the comparison does not use. |

## Provenance

Obtained 2026-09-05 from the `sgp4` Python package, which redistributes
Vallado's files unmodified under the MIT licence:

```
https://raw.githubusercontent.com/brandon-rhodes/python-sgp4/master/sgp4/SGP4-VER.TLE
https://raw.githubusercontent.com/brandon-rhodes/python-sgp4/master/sgp4/tcppver.out
```

```
SHA-256  d246d1d9d768ace445a38a965713fa9ba52d80fd8a41a0502ff83d7acffe2881  SGP4-VER.TLE
SHA-256  687bf28dbe52df86e8e60ab5cb4a08d1aa3dbcaf4e63b1f7ab95f044fbe3833b  tcppver.out
```

Checked in rather than fetched at test time on purpose: a verification suite
that only runs when a third-party host is up is not a verification suite. Both
files are byte-for-byte as obtained — including `SGP4-VER.TLE`'s CRLF line
endings and the five bad checksums noted below.

## Two things about the data that will otherwise look like bugs

**Satellite numbers are zero-padded in the TLEs and not in the output.** The
element set says `00005`, the reference block says `5`. Matching them
literally silently drops the first six cases.

**Five lines carry an incorrect modulo-10 checksum**, all on the three
hand-constructed error-code cases:

```
1 33333  says 4, computes 2      2 33333  says 8, computes 0
1 33334  says 9, computes 6
1 33335  says 0, computes 3      2 33335  says 1, computes 7
```

The other 61 lines are correct, which is what makes them a fair test of
astrogo's own checksum implementation. These three cases exist to exercise
SGP4's error returns, not its arithmetic, and `NewFromTLE` refuses them before
that — see the suite, which asserts exactly that rather than skipping quietly.
