---
type: Fixed
pr: 617
---
**`catalog.Resolver` never matched one provider's ID against another provider's alias**, though its alias match is documented to. It indexed IDs and aliases in separate namespaces, so MAST's `"M  31"` and SIMBAD's M31, whose aliases include `"M 31"`, came back from `Search` as two objects when neither had a position. IDs and aliases now share one normalized key, as in `catalog/xmatch`.
