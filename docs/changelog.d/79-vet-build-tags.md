---
type: Fixed
pr: 79
---
**CI never compiled the build-tagged test files.** 59 of them — network,
validation and integration — were built only by the weekly Validation
workflow, so a tagged file that did not compile reached `main` green, and one
did. `go vet` with all three tags now runs on every pull request.
