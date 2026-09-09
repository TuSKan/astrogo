---
type: Fixed
pr: 250
---
`remote`'s download lock is now genuinely exclusive within a process. It relied
on `fileblob`'s `IfNotExist` being mutex-guarded, which the pinned driver does
not do — measured, 8 goroutines on one bucket produced 51 rounds in 200 with
two or more simultaneous holders (#250).
