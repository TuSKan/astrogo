---
type: Fixed
pr: 976
---
**`catalog/sbdb`'s truncation test no longer fails when a request is retried.** It required exactly four requests over two calls, so a request retried after a timeout failed it under load. It now requires each call to reach the server for both kinds of body, which is what "a cut list is not cached" means. A retry cannot hide a cached answer.
