---
type: Changed
pr: 1237
---
**`remote`'s API transport is `net/http`; `resty.dev/v3` is no longer a dependency.** Retries, the whole-call timeout and POST idempotency behave as before. A 429 or 503's `Retry-After` sets the wait between attempts, and a wait that would outlast the call's timeout ends the retries at once instead of running out the clock.
