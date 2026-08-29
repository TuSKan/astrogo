<!--
docs/PULL_REQUESTS.md has the full workflow. The headings below are the ones
that carry weight in this repository — a body that answers them is a body a
reviewer can act on. Delete any that genuinely do not apply.
-->

## What was wrong

<!-- The defect or gap, concretely. If it was silent, say what made it silent. -->

## What changed

<!-- The fix, and any design choice worth arguing. If you rejected an
     alternative, one sentence on why. -->

## Measured

<!-- Numbers, not adjectives. Before and after. If a tolerance moved, say what
     the bound is derived from — the smallest fault worth catching, not the
     largest residual observed. -->

## What this does not fix

<!-- Anything you found and deliberately left. Leaving a bug alone is fine;
     leaving it unmentioned is not. Name the decision if the fix needs one. -->

## Verification

<!-- Which gates ran. At minimum:

     gofmt -l . && go build ./... && go vet ./...
     go test ./...
     golangci-lint run
     golangci-lint run --build-tags="integration,network,validation"

     Both linter runs, please — CI lints untagged, and a helper used only from
     a tagged file reads as unused there. See docs/PULL_REQUESTS.md §5.

     If you reproduced a pre-existing failure on main, say so, so nobody
     assumes this change caused it. -->

---

- [ ] A changelog fragment in `docs/changelog.d/` (or: this change is not user-visible)
- [ ] New exported symbols have tests in this change
- [ ] `Closes #NN.` on its own line above, if this fixes a filed issue
