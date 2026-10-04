---
type: Fixed
pr: 446
---
**`remote.GetFile` checks download consent before waiting for another
process's download lock**: a caller who could never download was held by the
lock for up to half an hour, and with it every lazy EOP lookup in the process
(#424).
