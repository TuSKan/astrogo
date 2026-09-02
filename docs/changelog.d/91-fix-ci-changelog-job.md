---
type: Fixed
pr: 91
---
**CI has been failing to start since #82.** The changelog-fragment job's shell
carried a literal newline inside a `printf` format string, which ended the YAML
block scalar early and made the whole workflow unparseable — so every run since
that merge failed before any job began.
