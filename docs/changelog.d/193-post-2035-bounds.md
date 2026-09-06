---
type: Changed
pr: 193
---
**The `±0.9 s` bound on UT1−UTC is now stated as borrowed rather than intrinsic.** It
holds only because leap seconds keep it there, and CGPM Resolution 4 (2022) ends them by
2035 — after which the zero-EOP degradation grows without bound. The `time` package doc
and the EOP warning both say so; the data path is unaffected (#147).
