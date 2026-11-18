---
type: Added
pr: 1245
---
**A merged `resolve.Target` says why its records are one object.** `Target.Sources` names each provider record it was built from, and `Target.Associations` gives, for each join, the identifier both records carry or the separation that joined them, with the provider that asserted it; a group kept apart as ambiguous says so. New in `catalog/resolve`: `SourceRecord`, `Identifier`, `ParseIdentifier`, `Identifiers` and `Association`.
