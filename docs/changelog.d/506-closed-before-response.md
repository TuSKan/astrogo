---
type: Fixed
pr: 506
---
**A network test failed instead of skipping when the server dropped the
connection before answering**: `Unreachable` now counts an `EOF` from
`http.Client.Do`, and a reset on Windows, where `WSAECONNRESET` never matched
`syscall.ECONNRESET`. A body cut short by a server that answered still fails
(#505).
