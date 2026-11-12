---
type: Fixed
pr: 1093
---
**`airglow.Fetch` and `AlmanacAt` no longer close `remote.Default()`**: each closed the process-wide client on the way out,
which dropped every endpoint's transport, so the next request to any service opened a new connection and restarted its
pacing interval.
