---
type: Fixed
pr: 949
---
**A `no-changelog` label added just after a push no longer leaves the Changelog Fragment check stuck red.** The check now reads the pull request's labels when it runs, not the labels recorded in the event that started it. Each new run also cancels the one in progress, so a re-run, or the newest run, sees the labels as they are.
