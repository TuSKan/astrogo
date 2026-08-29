---
type: Fixed
pr: 59
---
**CI now runs on every pull request, not only those targeting `main`.** A stacked pull request matched no branch filter and so ran no checks at all, which reads as "nothing to report" rather than "nothing was run".
