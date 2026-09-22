---
type: Changed — BREAKING
pr: 363
---
**`Time.Add` and `Time.Sub` take and return a `unit.Duration`**, replacing four
methods with two: `Add(time.Duration)`/`AddDays(float64)` and
`Sub() time.Duration`/`SubDays() float64` are gone.

**`Sub` no longer has a ceiling.** It returned an int64 nanosecond count, which
saturates just past ±292 years — well inside the range this library supports —
and `SubDays` existed alongside it solely because a saturated maximum is not an
answer. A float64 second count has neither problem, so there is one method.

Two consequences worth knowing:

- `Sub` no longer quantises to whole nanoseconds. A scale round trip leaves a
  few picoseconds of residue that integer rounding used to absorb; that
  rounding was an artifact of the type, not a measurement. Tests asserting
  exact equality on a round trip need a tolerance.
- Durations render as `1 s` and `60 min` rather than `1s` and `1h0m0s`, since
  `unit` puts a space before the symbol. A value a hair under a threshold shows
  in the smaller unit — an interval a few picoseconds under an hour is
  `60 min`, because `String` picks its unit from the value it holds rather than
  the one it rounds to.

Solver parameters follow the same type, because they are search intervals over
an ephemeris that meet an epoch directly: `Solver.Tolerance`,
`EventSolver.Step`, `NewEventSolver`, `ObservableWindows`' step and
`plan.WithStep`.

Scheduling and observing quantities keep `time.Duration` — `Block.Duration`,
`SetupTime`, `FilterChangePenalty`, `Cadence.MinInterval`,
`SatellitePass.Duration`, `Window.Duration` — since they are sub-day, written
as `30*time.Second`, and formatted by the standard library.
