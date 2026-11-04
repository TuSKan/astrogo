---
type: Fixed
pr: 995
---
**`TestComponentsAreConcurrencySafe` fails when a concurrent call does.** Its goroutines returned an `AddRadiance` error unreported, though the serial pass had succeeded with the same inputs, so a component that failed only under concurrency passed the test. A planted fault that errors on overlapping calls passed it before and fails it now.
