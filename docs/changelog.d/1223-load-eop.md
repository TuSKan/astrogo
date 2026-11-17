---
type: Added
pr: 1223
---
**`time.LoadEOP(ctx)`** loads Earth orientation when the caller chooses, under its context, and returns an error when no bulletin can be had, where the lazy load can only degrade to zero EOP with a warning. **`coord.Context.EOPSource`** reports what a Context's instant was placed with: the model's source, and whether the instant is inside the bulletin.
