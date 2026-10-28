---
type: Changed
pr: 889
---
**The Race Detection job runs on two runners and says where its time goes**: `plan` on one, every other
package but `internal/docsguard` on the other, with each shard's slowest tests in its summary; `kepler`'s
oracle sweep is thinner under `-short`. About 4 minutes, from 7.
