---
type: Fixed
pr: 188
---
**Four places where the documentation contradicted the code** (#119): the ROADMAP said
`LimitingMagnitudeConstraint` was removed while a differently-shaped one exists;
`plan/events.go` credited SOFA for a rise/set threshold that hardcodes the conventional
34′; `.golangci.yml` still described a go-cloud fork `replace` that `go.mod` has not
carried since the remote rebuild; and the `cams`, `kepler` and `xmatch` package synopses
were paragraphs, so pkg.go.dev rendered their directory listings as walls. The ROADMAP also still listed limiting magnitude as unbuilt while `skybrightness/plan.Imaging` implements it.
