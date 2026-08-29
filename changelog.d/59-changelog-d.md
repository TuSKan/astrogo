---
type: Changed
pr: 59
---
**Changelog entries now live one-per-file in `changelog.d/`**, assembled into `CHANGELOG.md` at release time. `CHANGELOG.md` was the only file five of eight pull requests conflicted on in a single batch — never the code — and resolving one textually can silently re-file an entry under the wrong heading, since merge markers carry no section information.
