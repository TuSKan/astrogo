#!/usr/bin/env bash
#
# Trial-merge every open pull request into the commit that was just pushed to
# main, and check the result still compiles.
#
# ── Why this exists ──────────────────────────────────────────────────────────
#
# GitHub tests a pull request against refs/pull/N/merge, which is the PR merged
# into its base — so a PR's own checks already cover the merged result. What
# they do not cover is main moving afterwards. Those checks are not re-run, so
# a PR stays green while the branch it would merge into changes underneath it.
#
# PR #169 is the case. It edited a line in plan.LookAngle. PR #165 merged first
# and rewrote that function, deleting the variable #169's line referred to. Git
# merged the two without a single conflict marker, because they touched
# different lines — and the result did not compile. It was caught only because
# a later push to #169 happened to re-trigger its checks; left alone it would
# have sat green and broken main on merge.
#
# A textual merge succeeding says nothing about whether the result builds. This
# is the job that asks.
#
# ── Scope ────────────────────────────────────────────────────────────────────
#
# Build and vet, not the full test suite: this runs once per push to main and
# multiplies by the number of open PRs, and a semantic conflict of this kind is
# a compile error essentially every time. The PR's own checks remain the place
# where behaviour is tested.
#
# A PR from a fork is skipped — its head is not a ref in this repository, and
# a fork PR cannot be trial-merged without fetching untrusted code into a job
# that holds a token.
set -uo pipefail

base_sha="$(git rev-parse HEAD)"
broken=()
checked=0
skipped=0

# --json/--jq keeps this to one API call regardless of how many PRs are open.
prs="$(gh pr list --state open --base main --limit 100 \
  --json number,headRefName,isCrossRepository \
  --jq '.[] | [.number, .headRefName, (.isCrossRepository|tostring)] | @tsv')"

if [ -z "$prs" ]; then
  echo "No open pull requests targeting main."
  exit 0
fi

while IFS=$'\t' read -r number head cross; do
  [ -z "${number:-}" ] && continue

  if [ "$cross" = "true" ]; then
    echo "── #$number ($head): skipped, from a fork"
    skipped=$((skipped + 1))
    continue
  fi

  echo "── #$number ($head)"

  if ! git fetch --quiet origin "$head"; then
    echo "   could not fetch the head ref; skipping"
    skipped=$((skipped + 1))
    continue
  fi

  if ! git -c user.name=ci -c user.email=ci@local merge --no-edit --quiet FETCH_HEAD >/dev/null 2>&1; then
    # A textual conflict is the PR author's to resolve and GitHub already
    # shows it, so it is reported but not counted as a break this job found.
    echo "   textual conflict with main — GitHub already reports this"
    git merge --abort >/dev/null 2>&1 || true
    git reset --hard --quiet "$base_sha"
    git clean -qfd
    skipped=$((skipped + 1))
    continue
  fi

  checked=$((checked + 1))

  # examples/ is a separate module (#124), so `./...` stops before it. It is
  # built here for the same reason the root module is: it reaches the library
  # through `replace ../`, which means a merged API change breaks it exactly
  # like any other consumer — and this job exists to find the consumer that
  # was green against an older main. Skipped when the trial-merged tree has no
  # examples module, so this script still works against a base predating it.
  examples_build=(true)
  if [ -f examples/go.mod ]; then
    examples_build=(go -C examples build ./...)
  fi

  if out="$(go build ./... 2>&1 && go vet ./... 2>&1 && "${examples_build[@]}" 2>&1)"; then
    echo "   builds against main"
  else
    echo "   DOES NOT BUILD against main:"
    echo "$out" | sed 's/^/     /'
    broken+=("#$number ($head)")
  fi

  git reset --hard --quiet "$base_sha"
  git clean -qfd
done <<< "$prs"

echo
echo "$checked pull request(s) trial-merged, $skipped skipped."

if [ ${#broken[@]} -ne 0 ]; then
  echo
  echo "These open pull requests merge cleanly but no longer build against main:"
  printf '  %s\n' "${broken[@]}"
  echo
  echo "Each needs a rebase onto main and a re-run of its own checks. Their"
  echo "existing green checks were computed against an older main and no"
  echo "longer mean anything."
  exit 1
fi
