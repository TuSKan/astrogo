---
type: Fixed
pr: 985
---
**`sbdb` and `vizier` tests that record their requests no longer fail when one is retried.** Each compared the exact list a test server saw, and a request retried after a timeout appears in it twice. They now compare the list with back-to-back repeats collapsed, which keeps the order each checks.
