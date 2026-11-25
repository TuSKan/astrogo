---
type: Fixed
pr: 1268
---
**`TestVisibleTonight_PlanetaryMoons` failed on every satellite kernel its own download policy refused.** A policy's refusal wraps both `remote.ErrDownloadDenied` and the policy's reason, and the test accepted only the first.
