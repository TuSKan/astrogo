---
type: Fixed
pr: 1154
---
`plan.VisibleTonight` lists an object once when two sources carry it, as SIMBAD and OpenNGC both carry Alnilam. Targets merge by a shared identifier through the new `resolve.IdentityKeys`, never by position, and `catalog.Resolver` and `xmatch.Match` match by the same keys.
