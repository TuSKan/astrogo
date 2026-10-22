---
type: Fixed
pr: 847
---
**`PostForm` and `PostJSON` were never retried**, whatever the `RetryPolicy`
said, so one slow SIMBAD answer failed a TAP query. An endpoint registered
`IdempotentPost` now has its POSTs retried; every TAP and JSON query endpoint
is, and `GaiaAIPAsync`, whose POST creates a job, is not.
