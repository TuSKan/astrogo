# Opening a pull request

How to get a change into astrogo, and the things that have actually gone wrong
while doing it. Everything below is written from real failures in this
repository — none of it is generic advice.

[CONTRIBUTING.md](../CONTRIBUTING.md) covers the project's goals, architecture
and testing philosophy, and is where to start. This is the operational
companion to its "Pull Request Process" section.

The short version: **branch, prove it, write a changelog fragment, run the full
gate, then open a pull request whose body says what you measured.**

---

## 1. Branch first, always

Never commit on `main`. Branch names follow the change:

| prefix | for |
| :--- | :--- |
| `fix/` | a defect in shipped behaviour |
| `feat/` | new capability |
| `test/` | coverage or a new validation suite |
| `docs/` | documentation and its guards |
| `chore/` | releases, tooling, CI |
| `refactor/` | structure, no behaviour change |

On a branch you created, commit and push freely. **Opening a pull request needs
an explicit instruction, and merging always does.**

---

## 2. Measure before you design

This repository's whole argument is that its numbers can be trusted, and the
practice that follows from it is: get a number *before* choosing an approach.

Two examples from recent work.

**The EOP dependency inversion** began by stubbing the import out and rebuilding
to see whether the payoff was real — 19.4 MB against 2.5 MB — so the design was
chosen against a measurement rather than a hope.

**A small-body accuracy contract** was first written at `1e-9 AU`, matching the
planetary path. Measured, that had **4,560× headroom**: a bound that cannot fail
for the reason it exists. It is `1e-11 AU` now, roughly 45× the observed float
scatter.

If you are about to write a tolerance, measure first, then derive the bound from
*the smallest fault worth catching* — never from what you happened to observe.
See [`internal/metrology`](../internal/metrology) for the contract-versus-measured
split this rests on.

---

## 3. Test what you add

Every new exported symbol ships with tests **in the same change**. An untested
front door is the worst one to leave: a broken constructor or option does not
fail where it is written, it fails somewhere with no idea it was involved.

A few conventions that are not obvious:

- **Put the test where the code is.** A test in package `foo` covers `foo`.
  Coverage is not attributed across packages, so a test for `ephemeris/core`
  living in package `ephemeris` leaves `core` at zero.
- **Prefer asserting the doc comment's own claim.** If a comment says "no named
  body returns an error from here", that is a test, and it is the kind of
  statement that quietly stops being true.
- **Bounds beat point values for physical quantities**, but a bound spanning two
  orders of magnitude is not a test. `0.1 AU < |r| < 5.0 AU` was the only
  assertion on a small-body position for a long time, and it could not tell a
  correct answer from one wrong by an astronomical unit.

---

## 4. Write a changelog fragment

One file per entry in [`docs/changelog.d/`](changelog.d/README.md), assembled
into `CHANGELOG.md` at release time:

```markdown
---
type: Fixed
pr: 61
---
**A thing.** Why it mattered, in one to three lines.
```

`type` is a [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) section:
`Added`, `Changed`, `Changed — BREAKING`, `Deprecated`, `Removed`, `Fixed`,
`Security`.

**Why fragments rather than editing `CHANGELOG.md`.** In one batch of parallel
work, five of eight pull requests conflicted — every one of them on
`CHANGELOG.md` alone, never on code. Resolving such a conflict textually is not
harmless: merge markers carry no information about which heading a bullet
belongs under, so an entry inherits whichever heading happens to precede it. An
`Added` entry silently became a `Fixed` one, in a diff that looked like
reordering. A file per entry cannot collide, and carries its own section.

`go test ./internal/changelog/` validates every checked-in fragment, so a
malformed one fails the pull request that added it rather than the release weeks
later.

Deep forensic detail — root cause, before-and-after numbers, what you refuted —
belongs in the pull request body or the code's own doc comment. **The changelog
is an index, not the record.**

---

## 5. Run the full gate

```bash
gofmt -l . && go build ./... && go vet ./... && go mod tidy
go test ./...
go test -race -short -count=1 ./...
golangci-lint run
golangci-lint run --build-tags="integration,network,validation"
```

**Run the linter twice, and understand why.** CI runs `golangci-lint` with **no
build tags**. A helper used only from a `network`-tagged file is, to that run, a
symbol with no consumers — and the whole file reports as unused. A change that
passes with tags and fails without them is the single most common way a green
local run turns red in CI. A declaration shared by two differently-tagged files
needs a constraint covering both, e.g. `//go:build validation || network`.

For a change touching external services, also run the relevant tagged suite:

```bash
go test -tags=network -count=1 ./ephemeris/...
go test -tags=validation -count=1 ./...
```

---

## 6. Linters that will bite

`.golangci.yml` runs `default: all` with documented disables. Do not weaken it
to pass. The ones that come up repeatedly:

| linter | what it catches here |
| :--- | :--- |
| `nolintlint` | a `//nolint` nobody needs. Adding one "just in case" fails. |
| `funcorder` | unexported methods after exported ones; constructors before methods. |
| `err113` | `errors.New` inline in a test table — hoist it to a package-level `var`. |
| `nilerr` | returning `nil` when an error is non-nil. Usually means an error is being used as a *predicate*: extract a `(T, bool)` helper and say so. |
| `wsl_v5` | a blank line before `if` after a statement. |
| `exhaustive` | a switch over a typed enum missing a case — including constants that are not really members, which is a hint the constant should be untyped. |
| `gosec` G115 | `int → uint32` conversions. A preceding range check usually satisfies it; if it does, do not also add a `//nolint`, because `nolintlint` will then flag that. |

**`//nolint` needs a scope and a reason**, both. `//nolint:gochecknoglobals // the
canonical section order, read-only` is fine; a bare `//nolint` is not.

---

## 7. External services must not fail the build

Network tests skip when an endpoint is unreachable — never fail CI for someone
else's downtime. But **reachability is not health**: a host answering
`500 Internal Server Error` opens a socket perfectly well, and a CelesTrak
outage once failed pull requests that had never touched the network.

Use `testutil.SkipOnUpstreamFailure(t, err)`, which classifies the *error*:

- **Skips**: 5xx, 429, 408, timeouts, connection reset, broken pipe, truncated body.
- **Still fails**: 400, 401, 403, 404, decode errors — because those mean
  *astrogo built a bad request*, which is exactly what the test exists to catch.

Keep `t.Fatal` for wrong data from a service that answered correctly.

---

## 8. Generated artefacts are `-update`-gated

The Horizons corpus and the accuracy table are regenerated by a test behind a
flag, never as a side effect of running the suite:

```bash
go test -tags=network -run TestGenerateCorpus ./ephemeris/jpl/validation/ -args -update-corpus
go test -tags=validation -run TestRenderAccuracyReport ./internal/metrology/ -args -update-accuracy
go test ./internal/changelog/ -run TestAssembleRelease -update -release-version X.Y.Z
```

**Never regenerate golden data because a test started failing.** Read the diff
first and understand every number that moved. A corpus once drifted because it
sampled an epoch in the *future*, where the reference depends on predicted Earth
orientation — accepting that diff would have frozen a prediction as a reference
and quietly widened every tolerance downstream.

Equally: do not publish a generated table you have not looked at. An accuracy
table was once rendered with three rows reading `NOT VERIFIED — JPL Horizons is
unreachable`, in a run where Horizons had answered fine moments earlier. Retried
individually they all verified; publishing the first version would have put a
false claim into the document the whole exercise exists to make trustworthy.

---

## 9. Deprecating a public symbol

Mark it with a **trailing** `// Deprecated: use X instead.` paragraph — after the
description, which is both the Go convention and what `revive` enforces.
Pre-1.0, a deprecated symbol survives **at least two minor releases**.

`staticcheck`'s SA1019 runs, so deprecating a symbol means migrating every
internal caller in the same change.

---

## 10. The pull request body

Bodies here are long, and they are long for a reason: they carry the reasoning
that does not belong in a commit message or a changelog. What earns its place:

- **The measurement.** "19.4 MB → 2.5 MB", not "significantly smaller".
- **What you rejected and why.** A separate module was considered for the EOP
  work and rejected in one sentence — moving code across a module boundary does
  not remove a dependency edge.
- **What you got wrong on the way.** A first attempt at a corrected date was
  still inside the unsettled window, and the guard caught it. That is the
  argument for the guard, in one line.
- **What you did not fix, and why.** Leaving a bug alone is fine; leaving it
  *unmentioned* is not. If the fix needs a design decision that is not yours to
  make, say so and name the decision.
- **Verification, concretely.** Which gates ran, which suites, and any
  pre-existing failure you reproduced on `main` so a reviewer does not think you
  caused it.

Avoid: adjectives standing in for numbers, and a summary of the diff. The
reviewer can read the diff.

### Referencing issues

A commit that fixes a filed issue needs a real closing keyword on its own line
in the body: `Closes #NN.` A bare `(#NN)` in the title is the changelog citation
style and does **not** close anything. In a pull request body, repeat the keyword
per issue — `Closes #20, closes #21` — because GitHub does not chain one keyword
across a list.

---

## 11. What CI will tell you

| check | note |
| :--- | :--- |
| Lint and Test | three platforms; the untagged lint trap in §5 lands here |
| Race Detection | `-race -short` |
| Integration Tests | external APIs; see §7 before assuming it is your change |
| codecov/patch | 70% of changed lines |
| codecov/project | 70% floor, not a ratchet |

**`codecov/patch` can be structurally unsatisfiable.** Coverage is generated from
an *untagged* `go test ./...`, so code reachable only under a build tag can never
be counted. When that happens the answer is usually to extract the decision the
gate is pointing at into something testable offline — which is a better change
anyway — rather than to argue with the number.

---

## 12. Two git habits worth having

**`git diff main branch` is not what a merge does.** Two-dot diff shows the
branch against `main`'s current tip, so a branch cut before a big merge appears
to delete everything that landed meanwhile. Use `git diff main...branch`
(three dots) to see what the branch actually changes.

**A branch that looks unmerged may be fully merged.** `git cherry` and
`git branch --no-merged` compare patch identity, so anything rebased or squashed
reads as unmerged forever. Check by content — does the symbol exist on `main`? —
before concluding work was lost.
