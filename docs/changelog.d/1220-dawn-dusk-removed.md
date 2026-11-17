---
type: Removed
pr: 1220
---
**`plan.CivilDawnDusk` was removed, with `NauticalDawnDusk` and `AstronomicalDawnDusk`.** The three returned a window's first dawn and first dusk: called after dusk, a dawn and a dusk from two nights. Use `plan.NightOf`, or `plan.TwilightEvents` for a window's crossings.
