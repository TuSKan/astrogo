---
type: Fixed
pr: 924
---
**A pull request from a fork can now be merged.** The branch ruleset requires a CodeQL result, and CodeQL ran as GitHub's default setup, which skips pull requests from forks, so no contributor's PR could ever satisfy it. CodeQL now runs from a workflow in the repository, on every pull request, analyzing the same languages as before.
