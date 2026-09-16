---
type: Fixed
pr: 327
---
**A directory listing's entry metadata now matches a stat of the same name.**
On Windows a directory entry's cached `LastWriteTime` lags the child's own,
so an `fs.FS` returning those entries while serving `Stat` from a real stat
disagreed with itself — measured at 15 failures in 40 for `os.DirFS` as well,
so it is the standard library's behaviour there rather than astrogo's.
`remote/file`'s local backend reads each entry's metadata when it is asked for
instead, which `fs.DirEntry.Info` explicitly contemplates, and now passes
`fstest.TestFS` unfiltered: 0 failures in 40. Closes [#323].
