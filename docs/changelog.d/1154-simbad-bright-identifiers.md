---
type: Added
pr: 1154
---
`simbad.Provider.SearchBright` returns each object with every identifier SIMBAD lists for it as its aliases, fetched with one further request per 1,000 objects. If they cannot be fetched, the objects still come, followed by an error counting those without them.
