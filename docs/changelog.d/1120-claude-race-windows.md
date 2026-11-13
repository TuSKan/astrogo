---
type: Changed
pr: 1120
---
**CLAUDE.md says how to run the race detector on Windows**: cgo there needs a MinGW-target C compiler, and the
Commands section now gives the llvm-mingw install and the per-command `CC`/`CXX` that `go test -race` needs.
