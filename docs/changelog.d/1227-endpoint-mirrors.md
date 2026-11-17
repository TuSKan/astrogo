---
type: Added
pr: 1227
---
**`remote.Endpoint.Mirrors`:** an endpoint's mirrors are tried in order when its host is not serving: no answer, a 404, or a 5xx once the retries are spent. Any other answer is the answer. SIMBAD registers its CfA and IDIA TAP mirrors. `remote.SetURL` clears an endpoint's mirrors and `remote.Reset` restores them.
