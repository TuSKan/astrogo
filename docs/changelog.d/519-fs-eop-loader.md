---
type: Changed — BREAKING
pr: 519
---
**`time.FileEOPLoader`, which took an OS path, is replaced by
`time.FSEOPLoader{FS, Name}`.** Write `time.FSEOPLoader{FS: os.DirFS(dir),
Name: "finals2000A.data"}` where you wrote `time.FileEOPLoader(path)`. A
bulletin that is there but unreadable is now reported as itself rather than as
`time.ErrNoEOPData`.
