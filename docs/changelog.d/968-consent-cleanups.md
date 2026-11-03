---
type: Fixed
pr: 968
---
**Test cleanups no longer grant every endpoint unlimited download consent, and `solar.Open`'s consent gate is tested.** Three network tests cleaned up with `remote.EnableDownloads(0)`, which grants consent rather than revoking it, so every later test in the binary could download anything. They now restore what they captured. The solar consent test, which passed whatever `Open` returned, is replaced by offline tests that can fail. A new docsguard test fails on any cleanup that grants consent.
