---
type: Added
pr: 155
---
**A guard that compiles the README's Go blocks.** `docsguard` proved a cited
name is *declared*; it could not prove a program *builds* — which is how
`coord.NewContext(epoch, observer, atmosphere.Atmosphere{})` survived a release
under the sentence claiming every sample was compiled and run. All 17 blocks
are now type-checked, and four broken samples are fixed.
