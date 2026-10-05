---
type: Changed — BREAKING
pr: 504
---
**`plan.CrescentResult` carries every criterion's answer with the quantities it
read** (`CrescentVerdict`, `CrescentZone.Params`). `EvaluateAll`, the shared
`Params`, `Fatoohi` and `Gautschy` are gone, the last two having no traceable
source; `Schaefer` is `Fatoohi1998` and `Ilyas1984` is `Ilyas1983` (#503).
