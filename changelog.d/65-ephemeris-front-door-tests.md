---
type: Added
pr: 65
---
**Tests for the `ephemeris` front door**, which had none: every constructor and every option — `NewProvider`, `NewFromElements`, `NewElements`, `NewMovingBodyProvider`, `WithKernel`, `WithTLE`, `WithTimeInterval`, `WithKeplerBase` — sat at 0% while the arithmetic beneath them was well covered. The package goes from 31.3% to 81.2%, with no exported function left uncovered.
