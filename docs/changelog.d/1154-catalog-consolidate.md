---
type: Added
pr: 1154
---
`catalog.Consolidate` makes one target of each object a set of providers' `catalog.Record`s describe, under the measurement policy `catalog.Resolver` uses, with provenance, and leaves an ambiguous group unmerged. `resolve.IdentityKeys` reads each identifier in its naming scheme, so SIMBAD's `HD  37128` meets OpenNGC's `HD 037128`; `catalog.Resolver` and `xmatch.Match` match by it too.
