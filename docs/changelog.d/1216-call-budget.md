---
type: Changed
pr: 1216
---
An endpoint's `Timeout` bounds a whole `remote.Client` request, its retries and the reading of its body included, where it bounded each attempt: a host that gave no answer cost a call four timeouts.
