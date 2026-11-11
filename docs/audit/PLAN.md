# Plan to finish the audit

The [audit map](README.md) lists what each package has had and what is still missing. This plan closes the missing items in order of where defects are most likely to remain.

## When the audit is done

The map shows no item still missing, except waivers that have been decided and are recorded in [tools/judgments.py](tools/judgments.py) with their reason. A sweep that finds nothing counts: its packages go into `SWEPT_CLEAN` there, and the sweep is added to `SWEEPS`.

## How each item is worked

- One issue per finding, and one branch, pull request and changelog fragment per fix, as for every other change.
- Every new test is canaried: the defect it exists to catch is planted, and the test must fail on it before it is trusted.
- A check against an outside reference reads the reference from its source (a published table, a reference implementation run locally, a live service), never from memory.
- After each phase, the map is regenerated (`collect.py`, `build.py`, `render.py`) and the change is part of that phase's last pull request.

## Phases

### 1. The two large packages with a light audit

Packages this size with this few findings are where unfound defects are most likely, and checks against an outside implementation found #524, #526 and #527 in one afternoon.

- [ ] `ephemeris/satellite/sgp4`: 1,389 lines, 3 findings. Compare against Vallado's published SGP4 verification set, including the deep-space cases and the error cases the set exercises, and test non-finite element-set fields.
- [ ] `constellation`: 1,842 lines, 2 findings. Compare against astropy's constellation lookup over a dense grid, plus points on boundaries, at the poles and across the 0h/24h wrap, and test non-finite inputs.

### 2. Non-finite inputs

The remaining numeric packages with no finding about NaN or infinite inputs, swept as #1038 swept the rest of the module.

- [ ] `vector`
- [ ] `ephemeris/core`
- [ ] `ephemeris/jpl`
- [ ] `ephemeris/satellite`
- [ ] `catalog/simbad`, `catalog/vizier`, `catalog/gaia`: the parsers keep non-finite coordinates, magnitudes and epochs in the targets they return. File it, then fix it once the policy below is decided.

### 3. Fuzz targets

Each parser of outside data gets a target of its own, whose queries come from the parsed data rather than fixed inputs (the lesson of #1037), and joins the fuzzing table in `CLAUDE.md`.

- [ ] `catalog/fink`
- [ ] `catalog/mast`
- [ ] `catalog/sbdb`
- [ ] `catalog/xmatch`
- [ ] `skybrightness/dataset/viirs`

### 4. Coverage

Each of these packages is raised to at least 75% statement coverage. The uncovered lines are read first, since they are mostly error paths no test has taken, and each new test asserts behavior rather than only reaching a line.

- [ ] `skybrightness/dataset`, 59.5%
- [ ] `skybrightness/dataset/starlight`, 65.7%
- [ ] `skybrightness/dataset/airglow`, 70.6%
- [ ] `logging`, 71.4%
- [ ] `catalog/internal/posmatch`, 73.3%
- [ ] `catalog/mast`, 74.6%

### 5. Test infrastructure and untouched packages

These packages decide whether other tests can fail, so each bound they apply is canaried with the fault it exists to catch.

- [ ] `internal/metrology`: 633 lines, 1 finding.
- [ ] `internal/oracle`: 556 lines, 1 finding.
- [ ] `internal/refraction`: never touched by a finding.
- [ ] `internal/leakcheck`: never touched by a finding.

### 6. Loose ends

- [ ] #913: Epv00 evaluated again for an instant already asked for.
- [ ] The 13 closed bugs the map cannot attribute to a package: attribute each to the commit that fixed it, or record why none did.
- [ ] #132: confirm every remaining item from the two external reviews is closed or has its own issue.

### 7. Close-out

- [ ] Regenerate the map and confirm nothing is missing except recorded waivers.
- [ ] Cut a release with the audit's last fixes.

## Open decisions

To be discussed before the items that depend on them.

- **Non-finite values from catalog services** (phase 2): whether a row with a non-finite required field, such as RA or Dec, is dropped with an error, and whether optional fields may keep NaN to mean "absent", as VOTable nulls can arrive.
- **#115**, `plan`'s 356 exported symbols and the three packages named `plan`: an API design question rather than an audit finding. Schedule it before 1.0, or close it.
- **`plan`'s slew time conversion**: the float-to-Duration conversion overflows only below a slew rate of 2e-8 deg/s. Record it as a waiver, or add the guard.
