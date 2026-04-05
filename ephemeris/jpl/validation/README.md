# JPL Horizons Validation Fixtures

This directory houses the rigorous mathematical validation suites verifying AstroGo's internal pipelines against NASA's JPL Horizons system. 

Because hitting external planetary servers repeatedly introduces massive flakiness constraints into standard CI workflows, the entire directory is architected to utilize **Offline Regression Corpora**.

## Architecture & Build Tags

### 1. The Offline Regression Corpus (`//go:build validation`)
All core CI regression algorithms are separated into `regression_test.go` and require the `validation` build tag to execute.
To run the automated suite blindingly fast against the permanent exact-truth baseline stored locally on your hard-drive without internet:
```bash
go test -v -tags=validation ./ephemeris/jpl/validation/...
```

### 2. Live Dynamic Analysis (`//go:build network`)
Whenever we add new features or evaluate entirely new edge cases extending the `Astrometric -> Observed` transformations, we query NASA directly. These tests are quarantined by the `network` build tag.
To run live against Horizons:
```bash
go test -v -tags=network ./ephemeris/jpl/validation/...
```

### 3. Re-Baking the Corpus Baseline
If we wish to shift the permanent truth boundaries (e.g. extending our exact-truth fixtures by tracking extremely eccentric comets), you can autonomously wipe and re-download the JSON verification blocks by running the local Corpus Generator:
```bash
go test -v -tags=network -run TestGenerateCorpus ./ephemeris/jpl/validation/...
```

## Data Points

The current static offline dataset `corpus/horizons_edgecases.json` locks in truth measurements representing mathematical boundaries:
- **Mars** at Greenwich Point [Standard Equator/Earth Alignment]
- **The Moon** at Latitude 89° [Circumpolar Precision Shifts]
- **Jupiter** at extreme high-altitudes and 0° Latitude [Equatorial Bending]

## Discrepancies
Minor differences (sub-arcsecond limits) are explicitly monitored and expected due to:
- Sub-minute interpolations within NASA's real-time EOP matrix differ slightly from pure offline SOFA DUT1 smoothing matrices. 
- Tiny integration step variations between deeply-dynamic orbital centers in DE441 (Horizons API) and DE440 (Local kernel caches).
