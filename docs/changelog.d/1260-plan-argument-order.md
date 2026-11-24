---
type: Changed — BREAKING
pr: 1260
---
**`plan` takes its arguments in one order: subjects, the site, the time or window, providers, then the rest**, and a `plan.Window` wherever it took a start and an end: `IsObservable(obj, site, t)`, `VisibilityEvents(target, site, w)`, `SunEvents(site, w, prov)`, `Constraint.Check(obj, site, t)`. `plan.Interval` was removed: `VisibleIntervals` and `Find` return `[]Window`, and `Find` takes its constraints last. `TimeWindow` is a `Window`, with `Start` and `End` for `From` and `To`.
