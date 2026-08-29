---
type: Added
pr: 70
---
**`internal/docsguard` now checks roadmap checkboxes against the code**, in both directions: an unchecked box whose symbol is already declared, and a tick whose symbol has been deleted, both fail the build. It found `resolve.Target.HasRadialVelocity` sitting unchecked while being declared, populated by `catalog/simbad`, preserved through the multi-provider merge and consumed by `plan` — finished work left looking open, which sends the next contributor to build it twice.
