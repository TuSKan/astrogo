---
type: Fixed
pr: 608
---
**Four `plan` Skyfield integration tests failed the build when NAIF was slow to deliver DE440s**, where every other kernel-backed test skips. They now skip through `requireKernel` on an upstream failure and stay fatal on anything else.
