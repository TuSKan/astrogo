---
type: Added
pr: 322
---
`remote/file` gains an `io/fs`-based storage core: `File` (`fs.File` +
`io.ReaderAt` + `io.Seeker`), the `CreateFS`/`RemoveFS`/`ContextFS` extension
interfaces, a scheme registry, and a `file://` backend. It sits alongside the
gocloud path for now. The new backend fixes [#315]: staging is named from the
process id and an atomic counter rather than a clock that does not advance on
Windows, and happens inside the tree so the rename cannot cross a volume.
