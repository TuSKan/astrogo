# Contributing to astrogo

First off, thank you for considering contributing to `astrogo`! 🌌

As a high-performance astronomy and observation-planning toolkit, `astrogo` places a high priority on **numerical correctness**, **performance (low allocations)**, and **clean package boundaries**.

Contributions are extremely welcome, particularly in:

- Numerical validation of algorithms
- Reference comparisons (e.g., cross-checking accuracy against Astropy or JPL Horizons)
- Performance and allocation-free path improvements
- Adding documentation and interactive examples

## Code of Conduct

By participating in this project, you agree to abide by our [Code of Conduct](./CODE_OF_CONDUCT.md).

## Getting Started

1. Fork the repository and create your branch from `main`.
2. Ensure you have the latest Go version installed.
3. If you've never used the JPL Horizons or SOFA algorithms, reviewing [docs/VALIDATION.md](docs/VALIDATION.md) is a great place to start to understand how we test accuracy.
4. Read [docs/PULL_REQUESTS.md](docs/PULL_REQUESTS.md) before your first pull request. It is the operational detail behind this document — the linter trap above, changelog fragments, `-update`-gated golden data, and what a reviewer here expects a pull request body to contain.

## Development Workflow

### 1. Build and Test

```bash
# Get dependencies
go mod tidy

# Run basic tests
go test ./...

# Run tests with race detection
go test -race ./...
```

### 2. Linting

We mandate `golangci-lint` to maintain our code quality. **Run it twice** —
CI lints with no build tags, so a helper used only from a `network`- or
`validation`-tagged file reads there as a symbol with no consumers, and the
whole file reports as unused. Passing with tags and failing without them is the
most common way a green local run turns red in CI.

```bash
golangci-lint run
golangci-lint run --build-tags="integration,network,validation"
```

## Architectural Guidelines

When submitting code, please ensure your architectural choices match the project's design goals:

- **No cyclic dependencies**: We enforce strict, clean unidirectional imports.
- **Explicit data models**: Use Go structures over magic mappings or empty interfaces.
- **No hidden state**: Avoid package-level variables and `init()` side effects. Do not introduce implicit unit conversions.
  - There is exactly one deliberate `init()` in the module, in `remote/eop.go`, which registers the EOP loader with `astrogo/time`. It exists so `time` need not import `remote`: that edge cost 17 MB of binary in cloud-storage and gRPC machinery for arithmetic that touches none of it. Inverting it is invisible to callers because download consent can only be granted through `remote`, so any program that could fetch EOP data already imports it. A second one needs an argument at least that good.
- **Minimal allocations**: For hot paths (transformations, loops), avoid heap allocations. Write batch-friendly computational paths where possible.

## Testing Philosophy

Astronomical calculations require strict numerical tolerances:

- **No silent assumptions**: Fail early instead of silently continuing with partial or ambiguous data.
- **Explicit tolerances**: Floating-point comparisons must be tested with explicit delta tolerances.
- **Test edge cases rigorously**: Be sure to consider behavior near poles, the horizon, angle wrapping (0 -> 360), epoch boundaries, and circumpolar/never-rising targets.

## Pull Request Process

See **[docs/PULL_REQUESTS.md](docs/PULL_REQUESTS.md)** for the full workflow,
including the failures this project has actually had and how to avoid repeating
them. In brief:

1. Provide a clear and descriptive PR title (e.g., `feat(ephemeris): add support for XYZ...` or `fix(transform): resolve pole wrapping bug`).
2. Clearly explain **why** the PR is needed.
3. Provide numerical proofs or benchmarks. Numbers, not adjectives — and where a
   tolerance moved, say what the new bound is derived from.
4. Add a changelog fragment in [`docs/changelog.d/`](docs/changelog.d/README.md)
   rather than editing `CHANGELOG.md`, so parallel pull requests cannot conflict
   on it.
5. Ship tests for every new exported symbol in the same change.
6. Ensure your PR passes all CI workflows (linting, tests, coverage).

## For AI Assistant Users (Claude, GitHub Copilot, ChatGPT, etc.)

If you're using AI tools to help with development:

1. Always review generated commit messages to remove any attribution
2. Ensure the message follows our commit style guide in CLAUDE.md
3. Remove any co-author tags automatically added by tools

Refer to [CLAUDE.md](CLAUDE.md) for complete style guidelines.
