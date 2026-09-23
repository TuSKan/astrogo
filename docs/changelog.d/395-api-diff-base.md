---
type: Fixed
pr: 395
---
**The API Diff check judges a pull request by its own changes.** It compared
the pull request merged into today's main with main as it was when the pull
request was opened, so every break merged in between was blamed on it: #392,
which only adds symbols, was told to declare #383's renames (#394).
