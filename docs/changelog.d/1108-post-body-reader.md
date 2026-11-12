---
type: Fixed
pr: 1108
---
**A POST answered before its body was read can no longer corrupt another request's body**: resty returned the body's
pooled buffer while net/http was still writing it, so a concurrent POST could send the wrong bytes or none. The race
detector reported it, and it was what the race shards had been failing on. `remote` now sends every POST body from a
reader of its own.
