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

- [x] `ephemeris/satellite/sgp4`: 1,389 lines, 3 findings. Compare against Vallado's published SGP4 verification set, including the deep-space cases and the error cases the set exercises, and test non-finite element-set fields.
  - The existing Vallado test was already strong.
  - A differential run against python-sgp4's C++ over 5,000 synthetic element sets came out clean, apart from libm cosine noise of one unit in the last place, near 180°.
  - Non-finite inputs found #1052: `At` never returned on a resonant orbit. Fixed in #1053.
  - Doc drift found #1054, fixed in #1084.
- [x] `constellation`: 1,842 lines, 2 findings. Compare against astropy's constellation lookup over a dense grid, plus points on boundaries, at the poles and across the 0h/24h wrap, and test non-finite inputs.
  - Against Roman's table, on aberration-free B1875 positions, 2 of 250,000 points disagree, each within 0.2″ of a boundary.
  - astropy's 83 disagreements are the 19″ of aberration it adds.
  - A NaN position answered Ursa Minor: #1055, fixed in #1056.

### 2. Non-finite inputs

The remaining numeric packages with no finding about NaN or infinite inputs, swept as #1038 swept the rest of the module.

- [x] `vector`: clean. NaN propagates. A magnitude beyond 1e±154 overflows, judged unreachable and recorded in `NOTED`.
- [x] `ephemeris/core`: clean, no float branches.
- [x] `ephemeris/jpl`: clean. A non-finite time ends in `ErrNoSegment`.
- [x] `ephemeris/satellite`: a NaN time became the year 1 through `ToGo`. Fixed with #1052, in #1053.
- [x] `catalog/simbad`, `catalog/vizier`, `catalog/gaia`: the parsers keep non-finite coordinates, magnitudes and epochs in the targets they return. File it, then fix it once the policy below is decided.
  - Filed as #1080 and fixed in #1081, for every catalog parser rather than only these three.
  - The fink and mast fuzz targets hold the result in #1087.

### 3. Fuzz targets

Each parser of outside data gets a target of its own, whose queries come from the parsed data rather than fixed inputs (the lesson of #1037), and joins the fuzzing table in `CLAUDE.md`.

- [x] `catalog/fink`: #1063.
  - An answer of `{}` resolved to a target with no ID: #1061.
  - A malformed Parquet schema panicked inside Arrow: #1062, now recovered.
- [x] `catalog/mast`: #1060, clean.
- [x] `catalog/sbdb`: #1059. A short row panicked `SearchBright`: #1058.
- [x] `catalog/xmatch`: no target, since it reads no bytes. It matches Targets that providers have already parsed, and it leaves `EXTERNAL` in `judgments.py`.
- [x] `skybrightness/dataset/viirs`: #1064, clean.

### 4. Coverage

Each of these packages is raised to at least 75% statement coverage. The uncovered lines are read first, since they are mostly error paths no test has taken, and each new test asserts behavior rather than only reaching a line.

- [x] `skybrightness/dataset`, 59.5%, to 81.9% in #1070. It measures 78.2% on main at 0e35ae51.
  - #1066: sums taken in map order. Fixed in #1072.
  - #1067 and #1068: a Spec checked only after the downloads. Fixed in #1074.
  - #1069: doc drift. Fixed in #1073.
- [x] `skybrightness/dataset/starlight`, 65.7%, to 77.2% in #1085, which fixes three findings:
  - #1077: a multi-band resume zeroed bands;
  - #1078: a non-finite Gaia flux;
  - #1079: `Fetch` lost completed batches.
- [x] `skybrightness/dataset/airglow`, 70.6%, to 84.1% in #1093.
  - #1093 fixes #1090: `Fetch` and `AlmanacAt` closed `remote.Default()`.
  - Reading it also found #1089, fixed in #1091: `remote.Client.Close` released no connections.
- [x] `logging`, 71.4%, to 100% in #1086, with no defect.
- [x] `catalog/internal/posmatch`, 73.3%, to 100% in #1094, with no defect. Its error returns looked unreachable, until a probe found that a parallax of 1e300″ makes SOFA's `Pmsafe` fail.
- [x] `catalog/mast`, 74.6%, to 95.5% in #1095.
  - The rest cannot be reached: a `json.Marshal` of fixed strings, and `MapCache.Set`'s error.
  - Reading it found #1088: an advertised cone search that always fails. The stub was removed in #1098.

### 5. Test infrastructure and untouched packages

These packages decide whether other tests can fail, so each bound they apply is canaried with the fault it exists to catch.

- [x] `internal/metrology`: 633 lines, 1 finding. Two more:
  - #1104: a suite that measured nothing reported `VERIFIED` and passed. Fixed in #1106.
  - #1109: `Baseline`'s regression check is never run: no baseline is recorded and nothing calls `Check`. It waits on a decision.
- [x] `internal/oracle`: 556 lines, 1 finding, and no defect. All 6,656 exact Kepler roots of the contract sweep agree with mpmath at 120 digits, to 2⁻²¹⁴ relative. The M = ±0 case its `ErrorOf` waives is checked exactly in the kepler contract test.
- [x] `internal/refraction`: never touched by a finding, and no defect. The transcribed SOFA series agrees with gofa's own `Atioq` and `Atoiq` to 1.4e-4 µas, which nothing compared before #1105.
- [x] `internal/leakcheck`: never touched by a finding until two:
  - #1099: `skybrightness` starts goroutines and ran no leak check. #1101 adds the check, and a docsguard rule that requires it in every package that starts one.
  - #1100: a profile header it could not read was reported as no leaks. Fixed in #1102.
- Found along the way: #1107, resty recycling a POST body's buffer while net/http was still writing it, the hidden cause of the race shards' failures. Fixed in #1108.

### 6. Loose ends

- [x] #913: Epv00 evaluated again for an instant already asked for.
  - #1110 gives every Kepler provider one SOFA base and its memo.
  - #1111 has a scheduling run compute each instant's Sun and Moon once.
  - #919, a bounded memo for instants a search comes back to, is open.
- [ ] The closed bugs the map cannot attribute to a package, listed in its summary: attribute each to the commit that fixed it, or record why none did (#1045 was fixed upstream, at JPL).
- [ ] #132: confirm every remaining item from the two external reviews is closed or has its own issue.

### 7. Close-out

- [ ] Regenerate the map and confirm nothing is missing except recorded waivers.
- [ ] Cut a release with the audit's last fixes.

## Open decisions

To be discussed before the items that depend on them.

- ~~**Non-finite values from catalog services**~~ (phase 2). **Decided 2026-11-11, #1080:**
  - A non-finite number in a catalog answer is read exactly as an empty or unparseable one, wherever it appears. A position that is non-finite or impossible is no position, and each parser does what it already does with one that will not parse.
  - A non-finite optional field is absent; nothing keeps NaN to mean "absent".
  - `plan.FromCatalog` refuses a non-finite coordinate and ignores non-finite kinematics, as a backstop.
- **#115**, `plan`'s 356 exported symbols and the three packages named `plan`: an API design question rather than an audit finding. Schedule it before 1.0, or close it.
- **`plan`'s slew time conversion**: the float-to-Duration conversion overflows only below a slew rate of 2e-8 deg/s. Record it as a waiver, or add the guard.
- ~~**#1088**, MAST's cone search~~ (phase 4). **Decided 2026-11-11:** the stub is removed, and `catalog/mast` no longer claims a cone search (#1098).
- **#1109**, metrology's `Baseline` (phase 5): its regression check exists but nothing records a baseline or calls `Check`. Wire it into the metrology suites, or remove it.
