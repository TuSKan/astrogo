---
type: Fixed
pr: 219
---
CLAUDE.md's fuzz throughput figures were unsupported and one was wrong by two
orders of magnitude: `fits.Read`'s "~4 exec/s" was the default 60 s minimize
time, not the parser, which measures ~800. Replaced with a corpus-controlled
table, the run-to-run spread that makes a single run not a measurement, and the
160× that `-fuzzminimizetime` alone decides (#200).
