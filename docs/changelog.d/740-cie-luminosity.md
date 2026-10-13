---
type: Changed — BREAKING
pr: 740
---
**The CIE luminous efficiency functions come from CIE itself, over https.** `remote.CVRLLuminosity`, a cleartext mirror unreachable since 2026-10-10, is replaced by `remote.CIELuminosity`, CIE's CC BY-SA 4.0 datasets. The tables are identical, so no luminance changes; grant downloads for the new endpoint.
