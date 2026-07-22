---
trigger: always_on
---

# Astrogo Engineering Rules

This repository is `github.com/TuSKan/astrogo`, a scientific Go astronomy library.

The agent must treat correctness, numerical reproducibility, and API stability as higher priority than cosmetic style changes.

## Git policy

**The agent must never run git commands.** No `git add`, `git commit`, `git tag`, `git push`, `git reset`, or any other git mutation. The user manages version control. The agent may run read-only commands (`git status`, `git diff`, `git log`) for inspection.

## Mandatory verification

Before reporting any task as complete, the agent must run the full verification gate:

```bash
go test -tags="integration,network,validation" ./...
go mod tidy ; go fmt; golangci-lint run
```

The agent must not say that a task is complete unless both commands pass.

If either command fails, the agent must:

1. Report the failing command.
2. Summarize the failure.
3. Fix the issue if it is related to the current changes.
4. Re-run the full verification gate.

Do not replace these commands with weaker alternatives such as:

- `go test ./...`
- `go test -short ./...`
- `golangci-lint run --fast-only`

Those are acceptable for quick local checks, but they are not the final acceptance gate.

## Preferred workflow

For every non-trivial code change:

1. Inspect the relevant package and tests.
2. Make the smallest correct change.
3. Run focused tests for the changed package.
4. Run the full verification gate:
   - `go test -tags="integration,network,validation" ./...`
   - `golangci-lint run`
5. Only then summarize the result.

## Go style policy

Use idiomatic, production-grade Go.

Prefer:

- explicit errors (static sentinels wrapped with `%w`, not dynamic `fmt.Errorf` strings)
- stable public APIs
- deterministic tests
- small focused changes
- table-driven tests where useful
- named return values when they document astronomical quantities such as `ra`, `dec`, `jd`, `az`, `alt`, or `dist`

Avoid:

- hidden global mutation
- unverified generated changes
- accidental public API churn
- broad rewrites unrelated to the task
- weakening tests to make them pass
- disabling linters without a precise documented reason

## Scientific code policy

This is a scientific/numerical codebase. The agent must preserve domain clarity.

Acceptable patterns include:

- physical constants
- astronomical coefficients
- Julian date constants
- NAIF IDs
- SOFA-style wrappers
- named astronomical quantities
- domain-specific short variables such as `ra`, `dec`, `jd`, `az`, `alt`, `dt`, `tt`, `ut1`

Do not "clean up" scientific formulas by abstracting constants unless the abstraction improves correctness or traceability.

Do not split published algorithms into many small helpers just to satisfy generic complexity style if that makes the algorithm harder to compare with the reference paper, SOFA routine, JPL/Horizons output, or validation fixture.

## Lint policy

The repository uses golangci-lint v2.

The agent must not:

- downgrade `.golangci.yml`
- remove linters only to make CI pass
- add broad `//nolint` comments without explanation
- use `//nolint` when a small code fix is better

A `//nolint` is acceptable only when:

- the linter is wrong for this scientific/domain-specific case;
- the suppression is as local as possible;
- the comment explains why.

Good example:

```go
//nolint:revive // Public duration unit constants; unit names are intentional in astrogo/time.
const Minute time.Duration = time.Minute
```

Bad example:

```go
//nolint
```

## Cross-platform policy

Tests must pass on Linux (CI), macOS (ARM64/Apple Silicon), and Windows.

- Floating-point comparisons must use safe tolerances that account for FMA/atan2 rounding differences across architectures.
- Prefer inequality bounds over exact equality for computed values near precision boundaries.
- When relaxing a tolerance, document why and reference the platform-specific behavior.

## Network test policy

Tests behind the `network` build tag depend on external APIs that may be transiently unreachable.

- Add a fast connectivity pre-check (TCP dial with ≤5s timeout) before slow API calls.
- Use `t.Skipf` when the endpoint is unreachable — never fail CI for external downtime.
- Keep `t.Fatal` for logic errors when the endpoint *is* reachable but returns wrong data.
- The portal frontend and API backend may be on different hosts; check the actual API host.

## Tests

The agent must preserve or improve test coverage for changed behavior.

For bug fixes, add or update a regression test unless impossible.

For numerical code, tests should prefer:

- known reference values
- explicit tolerances
- deterministic fixtures
- comparison against SOFA, JPL/Horizons, or documented astronomical references when available

Do not loosen tolerances without explaining why.

## Generated files

This codebase has no `go:generate` step, and no package uses `go:embed` — do
not reintroduce either. Every data source is obtained at runtime through
`remote.GetFile`: `catalog/openngc` fetches explicitly on `New()`; `time`'s
Earth Orientation Parameters load lazily and automatically on first
`Time.EOP()`/`Time.UTC()`/`Time.UT1()` query (pre-seeded disk cache, then a
`remote.EnableDownloads`-gated network fetch). Populate data by pre-seeding
the file or granting `remote.EnableDownloads`/`EnableAllDownloads` — never
by adding a generation/download tool.

## Completion contract

A task is complete only when:

1. The requested code/documentation change is implemented.
2. The full verification gate passed:
   - `go test -tags="integration,network,validation" ./...`
   - `golangci-lint run`
3. The final response includes:
   - changed files;
   - verification commands run;
   - result of each command;
   - any remaining risks or skipped checks.