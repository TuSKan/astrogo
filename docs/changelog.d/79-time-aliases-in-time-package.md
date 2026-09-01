---
type: Changed
pr: 79
---
**The `time` package's own tests no longer alias it.** Five external test
files carried `atime`, `astrotime` and `gotime` — the package that exists to
end the two-spellings-of-time problem was the last place still having it. The
guard now applies its alias rule inside `time/` too.
