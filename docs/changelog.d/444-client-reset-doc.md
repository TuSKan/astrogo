---
type: Fixed
pr: 444
---
**`remote.Client.Reset`'s doc says what it restores**: endpoints, consent,
offline mode and policy, leaving the data directory and API options, where it
promised all of `NewClient`'s state. A `catalog/mpcorb` test that trusted it
broke the tagged suite, and a docsguard check now catches the pattern (#443).
