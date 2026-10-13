---
type: Changed — BREAKING
pr: 740
---
**The CIE luminous efficiency functions come from CIE itself, over https.** The `CVRLLuminosity` endpoint, a cleartext mirror unreachable since 2026-10-10, no longer exists; `remote.CIELuminosity`, CIE's CC BY-SA 4.0 datasets, replaces it. The tables are identical, so no luminance changes; grant downloads for the new endpoint.
