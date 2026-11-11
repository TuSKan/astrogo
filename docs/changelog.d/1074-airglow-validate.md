---
type: Added
pr: 1074
---
**`airglow.Spec.Validate`** makes the checks `airglow.Fetch` makes before its first request, without asking SkyCalc,
so a caller gathering several datasets can refuse a bad airglow spec before fetching the others.
