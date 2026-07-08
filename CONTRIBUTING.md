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
3. If you've never used the JPL Horizons or SOFA algorithms, reviewing `VALIDATION.md` is a great place to start to understand how we test accuracy.

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
We mandate `golangci-lint` to maintain our code quality.
```bash
# Run the linter locally before submitting a PR
golangci-lint run ./...
```

## Architectural Guidelines

When submitting code, please ensure your architectural choices match the project's design goals:
- **No cyclic dependencies**: We enforce strict, clean unidirectional imports.
- **Explicit data models**: Use Go structures over magic mappings or empty interfaces.
- **No hidden state**: Avoid package-level variables and `init()` side effects. Do not introduce implicit unit conversions.
- **Minimal allocations**: For hot paths (transformations, loops), avoid heap allocations. Write batch-friendly computational paths where possible.

## Testing Philosophy

Astronomical calculations require strict numerical tolerances:
- **No silent assumptions**: Fail early instead of silently continuing with partial or ambiguous data.
- **Explicit tolerances**: Floating-point comparisons must be tested with explicit delta tolerances.
- **Test edge cases rigorously**: Be sure to consider behavior near poles, the horizon, angle wrapping (0 -> 360), epoch boundaries, and circumpolar/never-rising targets.

## Pull Request Process

1. Provide a clear and descriptive PR title (e.g., `feat(ephemeris): add support for XYZ...` or `fix(transform): resolve pole wrapping bug`).
2. Clearly explain **why** the PR is needed.
3. If applicable, provide numerical proofs or benchmarks demonstrating your changes.
4. Ensure your PR passes all CI workflows (linting, tests, coverage).

## For AI Assistant Users (Claude, GitHub Copilot, ChatGPT, etc.)

If you're using AI tools to help with development:

1. Always review generated commit messages to remove any attribution
2. Ensure the message follows our commit style guide in CLAUDE.md
3. Remove any co-author tags automatically added by tools

Refer to [CLAUDE.md](CLAUDE.md) for complete style guidelines.
