---
type: Fixed
pr: 88
---
**SIMBAD name resolution returned the wrong object for most deep-sky names.**
`Resolve("M87")` gave a source 70° away in Cassiopeia, `Resolve("M31")` a nova
inside the galaxy. The substring `LIKE '%name%'` is now an exact match against
the spellings SIMBAD stores, and an unknown name returns not-found rather than
the least-wrong of ten.
