---
type: Fixed
pr: 991
---
**Six tests no longer depend on the order they run in.** Two `remote` tests set API options on `Default()` itself, and neither `Reset` nor `Capture` restores those, so every later test in the binary inherited a 10 s timeout, an `Authorization` header or a policy that never retries. They now configure a clone. Four `time` tests assumed no EOP model had loaded yet, which held only when they ran before anything lazily loaded this machine's cached bulletin. They now call `ResetEOP` first. `go test -shuffle=on ./...` found them.
