---
type: Fixed
pr: 662
---
**The weekly validation tier had timed out in the GAMBONS comparisons every week since 31 August**, so it reported nothing. The `skybrightness` package spent 22.5 minutes asking IRSA, two seconds apart, for the same few hundred dust values every run, because a runner starts with an empty cache. The run now seeds that cache with IRSA's own answers, kept in the repository, and stays within its 15 minutes. `TestSFDMatchesIRSA`, which reads the same cache and had skipped in CI on every run, now runs.
