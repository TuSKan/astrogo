---
type: Fixed
pr: 622
---
**`catalog/sbdb` and `catalog/norad` reported "no such object" as a failure**, so a `catalog.Resolver` with either could never return `ErrNotFound`. SBDB's "specified object was not found" is now `resolve.ErrNotFound`, and a name several objects match is `resolve.ErrAmbiguous` unless one of them has the query as its designation. CelesTrak's 404 "No GP data found" is now an empty result.
