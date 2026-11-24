---
type: Fixed
pr: 1251
---
**CI:** the nightly network examples create their download cache when the workflow's cache comes back empty. Without it every example that downloads failed, and the nightly run with them.
