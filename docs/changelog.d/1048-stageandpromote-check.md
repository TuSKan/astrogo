---
type: Changed — BREAKING
pr: 1048
---
**`remote/file.StageAndPromote` checks the staged object by key**, `func(fs.FS, string) error`, instead of by stream, so a caller's reader can decide before a cached copy is replaced. `remote/file` is internal to `remote` and nothing outside it may import it, so no caller outside the module is affected.
