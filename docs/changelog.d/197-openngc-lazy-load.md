---
type: Changed
pr: 197
---
**`openngc.New` no longer fetches the catalog.** It did about 7 MB of I/O under
`context.Background()`, so `catalog.NewResolver(SIMBAD, OpenNGC)` blocked for two
seconds against a warm cache with nothing above it able to cancel. The load now
happens on the first query, under that caller's context, and a failed load is
retried rather than replayed for the life of the provider.
