#!/usr/bin/env bash
#
# A break in the exported API must be declared, not discovered downstream.
#
# # Why this is not the changelog-fragment job over again
#
# That job checks a fragment exists. It cannot check the fragment is true: today
# a "Changed — BREAKING" entry says a break happened because a human wrote it
# down, and a break with no entry says nothing at all. apidiff knows.
#
# So the rule is one-directional and deliberately so. An undeclared break fails.
# A declared one passes, whatever apidiff thinks — a fragment claiming a break
# that apidiff cannot see is usually a behavioural break rather than a
# signature one, and this script has no business calling that a mistake.
#
# # Why against the pull request's base rather than the last tag
#
# gorelease compares against a released version, which is the right question at
# release time and the wrong one here: nineteen minor releases in eight weeks
# means the diff against the last tag is everything anyone merged since, and a
# job that reports all of it on every pull request tells the author nothing
# about their own change. The base is what isolates it.
#
# Usage: api-diff.sh <base-sha> <pr-number>

set -euo pipefail

base_sha="${1:?usage: api-diff.sh <base-sha> <pr-number>}"
pr_number="${2:?usage: api-diff.sh <base-sha> <pr-number>}"

readonly module="github.com/TuSKan/astrogo"

work="$(mktemp -d)"
trap 'rm -rf "$work"; git worktree prune' EXIT

# Export data for the base, read from a worktree at that commit. apidiff has to
# run inside the directory holding the go.mod the module path belongs to, or it
# consults the module cache and answers about a published version instead.
git worktree add -q --detach "$work/base" "$base_sha"

# run captures apidiff's combined output, keeps its exit status, and drops only
# the per-package "Ignoring internal package" notes it writes to stderr.
#
# The exit status is checked rather than swallowed. apidiff returns zero when it
# finds changes, so a non-zero status means it could not read something — and a
# missing or truncated base export would otherwise be reported as the whole API
# having been removed, which is the one false alarm this job must never raise.
run() {
	local out="$1"
	shift

	if ! "$@" > "$out.raw" 2>&1; then
		echo "FAIL: apidiff could not compare the two trees."
		echo
		cat "$out.raw"
		exit 1
	fi

	grep -v '^Ignoring internal package ' "$out.raw" > "$out" || true
}

( cd "$work/base" && apidiff -m -w "$work/base.api" "$module" ) > "$work/export.raw" 2>&1 || {
	echo "FAIL: apidiff could not read the base API at $base_sha."
	echo
	cat "$work/export.raw"
	exit 1
}

# Both listings. The full one is what the author reads; the incompatible one is
# what decides the exit status, and it is computed separately rather than
# grepped out of the first, so a change in apidiff's formatting cannot quietly
# turn the gate off.
run "$work/all.txt" apidiff -m "$work/base.api" "$module"
run "$work/incompatible.txt" apidiff -m -incompatible "$work/base.api" "$module"

# apidiff counts `main` packages as API. They are not: nothing can import one,
# so nothing downstream can break when one moves, is renamed, or goes away.
#
# It matters because this repository has 32 of them. Moving examples/ into its
# own module (#124) removed every one from this module and apidiff reported 32
# incompatible removals, which would have forced a `Changed — BREAKING`
# changelog entry announcing a break no caller could observe — the gate
# demanding a false statement to stay green.
#
# The list comes from `go list` at the base commit rather than from a path
# pattern: a package is a command because its clause says `package main`, and
# guessing that from a directory name is how the next reorganisation slips
# through. Filtering the base's set is also the conservative direction — a
# package that is a command only at the head is still checked.
( cd "$work/base" && go list -f '{{if eq .Name "main"}}{{.ImportPath}}{{end}}' ./... ) \
	> "$work/commands.txt" 2>/dev/null || : > "$work/commands.txt"

if [ -s "$work/commands.txt" ]; then
	for listing in "$work/all.txt" "$work/incompatible.txt"; do
		grep -vFf <(sed 's/$/:/; s/^/package /' "$work/commands.txt") \
			"$listing" > "$listing.api" || true

		# Filtering every entry out of a section leaves apidiff's heading
		# standing over nothing, which reads as a finding with the detail
		# missing rather than as no finding. Drop a heading with no bullet
		# under it, so an emptied listing is genuinely empty and the
		# "None."/"No incompatible changes." branches below still fire.
		awk '
			/^- / {
				if (heading != "") { print heading; heading = "" }
				print
				next
			}
			/^[[:space:]]*$/ { next }
			/:$/             { heading = $0; next }
			                 { print }
		' "$listing.api" > "$listing"
	done
fi

echo "## Exported API changes against the base"
echo

if [ -s "$work/all.txt" ]; then
	cat "$work/all.txt"
else
	echo "None."
fi

echo

if [ ! -s "$work/incompatible.txt" ]; then
	echo "No incompatible changes."
	exit 0
fi

echo "## Incompatible changes"
echo
cat "$work/incompatible.txt"
echo

# A fragment for this pull request declaring the break. Matched on the type
# line rather than anywhere in the file, so prose mentioning the words does not
# satisfy the gate.
shopt -s nullglob
declared=0

for f in docs/changelog.d/"$pr_number"-*.md; do
	if grep -qE '^type:[[:space:]]*(Changed — BREAKING|Removed)[[:space:]]*$' "$f"; then
		echo "Declared in $f."
		declared=1
	fi
done

if [ "$declared" -eq 1 ]; then
	exit 0
fi

cat <<EOF

FAIL: the exported API changes incompatibly and no changelog fragment says so.

Add docs/changelog.d/${pr_number}-<slug>.md with:

    type: Changed — BREAKING

or "Removed" if the symbols are gone rather than reshaped, and say in the body
what a caller has to do. Pre-1.0 a break is allowed; going unannounced is not,
and the list above is the migration note somebody would otherwise have to
reconstruct from a downstream build failure.
EOF

exit 1
