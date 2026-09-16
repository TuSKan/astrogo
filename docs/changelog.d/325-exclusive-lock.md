---
type: Fixed
pr: 325
---
**The cross-process download lock is now exact.** It was gocloud's
`WriterOptions.IfNotExist`, which under `fileblob` was a Stat followed by a
Rename and could admit a second holder ([#241]). It is `O_CREATE|O_EXCL` inside
an `os.Root`, which the kernel makes indivisible, so a losing writer has exactly
one way to lose and it is `fs.ErrExist` — the three-code classifier and the
staging lock the lock needed around its own write are both gone.
