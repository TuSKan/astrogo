---
type: Fixed
pr: 1154
---
`plan.VisibleTonight` lists an object once when two sources carry it, as SIMBAD and OpenNGC both carry Alnilam and the LMC. The sources' records go through `catalog.Consolidate`, which merges by identity, never by position.
