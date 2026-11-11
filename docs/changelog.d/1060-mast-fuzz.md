---
type: Added
pr: 1060
---
**`catalog/mast` has a fuzz target, `FuzzResolveObject`**, which serves fuzzed name-resolution answers, JSON or
XML, through an `httptest` server and holds resolution to never panicking.
