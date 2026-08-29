---
type: Added
pr: 64
---
**`docs/VALIDATION.md`'s status table cites its evidence, and `internal/docsguard` checks the citations resolve.** 52 of its 53 rows said "validated" — undated, with no link to what establishes them — in the same document as generated rows carrying a contract, a measured distribution and a commit stamp. Each row now names a test file or a generated suite; a renamed file, a suite that stops being produced, or a measured suite no row points at fails the build. That last check immediately found small-body and SOFA-analytical ephemerides being measured with no row at all.
