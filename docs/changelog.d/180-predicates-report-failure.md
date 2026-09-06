---
type: Fixed
pr: 180
---
**Seven predicates could not report a failure, so an error read as "the answer
is no".** The scheduler's and visibility solver's bisection predicates returned
a bare `false` for a constraint that *could not be evaluated*, so a target
silently vanished from a schedule; `ObservableWindows` swallowed a failure
during refinement, moving a rise/set boundary rather than dropping it; and
`DiffuseGalacticLight.capFactor` treated a star map that could not answer as a
sightline with no starlight, quietly dropping the Toller cap. All now
propagate. `VisibleTonight` keeps skipping a candidate it cannot evaluate — its
documented contract — but reports each skip at `Warn` instead of silently (#177).
