---
type: Changed
pr: 732
---
**Three `coord` tests that guard against an input having no effect now fail below a floor rather than only at exactly zero.** A point's separation from itself is 2e-13″ on arm64, so an FK4 conversion that ignored its epoch would have passed on macOS; a `SetTime` that rebuilt inside the hour could have passed everywhere.
