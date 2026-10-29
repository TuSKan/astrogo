---
type: Fixed
pr: 930
---
**The docsguard check that keeps CI's Go pins in step with go.mod reads every workflow.** It read two of the five files that pin Go, so `govulncheck.yml`, `release.yml` and `codeql.yml` could fall behind go.mod unnoticed.
