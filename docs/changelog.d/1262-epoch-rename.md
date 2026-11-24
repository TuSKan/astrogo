---
type: Changed — BREAKING
pr: 1262
---
**astrogo's `time` package is now `epoch`** (`github.com/TuSKan/astrogo/epoch`), beside the standard library's `time` with no alias: `epoch.Time` is an astronomical instant, `epoch.UTC` a scale, and `time.UTC` Go's location. `NowUTC` is `epoch.Now`, and the package's error messages begin `epoch:`.
