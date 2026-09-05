---
type: Added
pr: 174
---
**A `logging` package replaces the three production writes to the global `log`
package**, whose output went wherever `log.SetOutput` last pointed. It is the
only package in astrogo that imports `log/slog`; everything else calls
`logging.Info`/`logging.Warn`, which name no slog type. Progress lines are
`Info` and discarded by default; the EOP-unavailable message is `Warn` and
still emitted, because `Time.EOP` has no error return and that line is the only
notice a caller gets that topocentric accuracy silently dropped to ~1 arcsec.
`logging.Set(nil)` restores the default, `slog.DiscardHandler` silences
everything (#108).
