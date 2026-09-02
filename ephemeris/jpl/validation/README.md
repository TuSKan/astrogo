# JPL Horizons validation

This directory compares astrogo's astrometric-to-topocentric pipeline against
NASA's JPL Horizons, both live and against a frozen corpus.

## Build tags

Nothing here runs under a plain `go test ./...`. The two tags split by what a
run needs, not by what it tests:

| tag | needs | what it does |
| :--- | :--- | :--- |
| `validation` | the checked-in corpus, no network | offline regression against frozen Horizons answers |
| `network` | a reachable `ssd.jpl.nasa.gov` | live comparison, and corpus regeneration |

```bash
go test -tags=validation ./ephemeris/jpl/validation/
go test -tags=network    ./ephemeris/jpl/validation/
```

Neither runs in ordinary CI. The scheduled `Validation` workflow runs both
weekly, and uploads each suite's result document as an artifact.

## The corpus

`corpus/horizons.json` holds **255 entries**: three bodies at five named
sites, across three sampling classes.

- **regular** — 120-day steps across three years, which is what catches
  secular drift.
- **boundary** — epochs straddling the 2017-01-01 leap second and J2000.0.
  Algorithms fail at their boundaries far more often than in the middle, and
  even sampling almost never lands on one.
- **adversarial** — the same epochs at a synthetic 78°N site, where
  hour-angle-to-azimuth projection is most extreme, and at the equator, where
  targets transit near the zenith and azimuth is ill-conditioned.
  `plan.KnownSites` cannot supply either: its extremes are Greenwich at 51
  north and Paranal at 25 south.

Each entry carries Horizons' geocentric state and its topocentric answer for
the same instant. The manifest records the query shape, the astrogo commit,
the sampling design, and — explicitly — what is *not* pinned and why.

The Sun, Mercury and the Moon are absent on purpose. Horizons returns a bare
HTTP 500 for those three under `CENTER='coord@399'`, reproducibly and
independently of astrogo; it is confirmed with curl in isolation and
documented in `observer_precision_test.go`.

## Regenerating it

Reference data is evidence. The generator therefore **writes nothing** unless
told to:

```bash
# Fetch, compare, and report what would change. Fails if anything would.
go test -tags=network -run TestGenerateCorpus ./ephemeris/jpl/validation/

# Accept the change and write it.
go test -tags=network -run TestGenerateCorpus ./ephemeris/jpl/validation/ -args -update-corpus
```

The summary names entries added, removed and changed, and the largest numeric
change — because "247 entries changed" is not reviewable and "3 entries
changed, worst 4.2e-06 degrees in azimuth at Saturn @ Paranal" is. Paste it
into the commit message.

**Never regenerate because a test started failing.** Find out why it failed
first. A corpus rewritten to make a suite pass is a suite that no longer
measures anything.

The whole run is about 54 requests, since the geocentric state does not depend
on the observing site and one vector series covers every site.

## What is measured, and what is not

Each suite reports its distribution — p50 through max, the signed bias, and
the scenario that produced the worst case — separately from the bound it
claims to satisfy. See [`internal/metrology`](../../../internal/metrology) for
why those are two different numbers.

`TestScientificStability` feeds Horizons' own geocentric state through a
linear mock provider, so **no kernel is read and the ephemeris is not under
test**. What is under test is everything downstream: light-time iteration, the
IAU 2006/2000A reduction chain, observer geodesy, and topocentric parallax.

Its intermediate place is measured but deliberately **not** contracted.
`eph.ApparentState` retards the whole geocentric vector, carrying light-time
and annual aberration together, while Horizons' astrometric column has
light-time only and its apparent column is referred to the true equinox of
date where `coord.Context.AstrometricToApparent` produces CIRS. Neither published
column is the quantity astrogo computes, so the gap is named and quantified —
Earth's motion over the light time, up to 21.2 arcsec — rather than hidden
under a widened tolerance. That test's doc comment has the full account.

## Known discrepancies

Sub-arcsecond differences are expected and monitored:

- astrogo interpolates DUT1 and polar motion from IERS `finals2000A`; Horizons
  uses its own series. This is the dominant term, and it is what the 3
  arcsecond contract is derived from.
- Horizons serves DE441 while the local cache is DE440. This does not reach
  `TestScientificStability`, which uses no kernel, but it does reach the live
  comparisons.
