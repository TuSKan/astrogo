---
type: Added
pr: 174
---
**`logging.Set(*slog.Logger)`** replaces the three production writes to the
global `log` package, whose output went wherever `log.SetOutput` last pointed.
Progress lines (a kernel downloading, an EOP table loading) are `Info` and
discarded by default; the EOP-unavailable message is `Warn` and still emitted,
because `Time.EOP` has no error return and that line is the only notice a
caller gets that topocentric accuracy silently dropped to ~1 arcsec. Pass a
`slog.DiscardHandler` for full silence, or an Info-level logger for the
progress lines. `TestNoLibraryCodeWritesToTheGlobalLog` keeps the next one from
being added (#108).
