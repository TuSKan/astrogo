# Releasing

astrogo releases every week, and out of band for security fixes and
regressions. The version number follows from the changelog, and every tag gets
a GitHub Release whose notes are that version's changelog section.

## Why on a schedule

A downstream that may not pin a commit can use only what is tagged, and an
untagged fix is no fix to it. One gap between tags ran to five weeks: over 300
merged pull requests, among them three security fixes and most of the bugs one
downstream user had reported, all on `main` and in no release (#644). A weekly
train bounds that wait at seven days without making anyone decide when "enough"
has accumulated.

## When

- **Every Monday**, after that week's validation and integration run
  (`pre-release.yml`, scheduled for Monday 04:43 UTC), cut a release if
  `docs/changelog.d` holds any fragment. A week with nothing merged makes no
  release.
- **Within a day, out of band**, for a `Security` fragment, or for a fix to
  something that worked in the previous release and broke in it. That is a
  patch release from `main` as it stands.

## Which version

The fragments in `docs/changelog.d` decide it. The project is pre-1.0, where a
minor release may break the API:

| fragments present | release |
| --- | --- |
| any of `Added`, `Changed`, `Changed — BREAKING`, `Deprecated`, `Removed` | minor, `0.X+1.0` |
| only `Fixed` and `Security` | patch, `0.X.Y+1` |

A bug fix that changes computed output is still a fix. Cross-check with the API
diff against the previous tag:

```bash
.github/scripts/api-diff.sh "$(git rev-parse vX.Y.Z)" 0
```

Read its report rather than its exit status. With no pull request to look up it
exits non-zero on any incompatible change, declared or not. An incompatible
change there while only `Fixed` and `Security` fragments are pending means a
fragment has the wrong type: correct the fragment rather than the version.

## Gates

The commit being tagged must have:

- **CI green** on all three platforms, including the race run.
- **The latest nightly network run green** (`pre-release.yml`), or each failure
  triaged as an upstream outage with an issue of its own. The network tier is
  the only thing that notices a catalogue service changing its schema.
- **That week's validation and integration run green.**
- **`govulncheck` clean.**

```bash
gh run list --workflow pre-release.yml --limit 7
gh run list --workflow govulncheck.yml --limit 1
```

## How

1. **Branch** `chore/release-vX.Y.Z` from `main`.
2. **Assemble the changelog**:

   ```bash
   go test ./internal/changelog/ -run TestAssembleRelease -update -release-version X.Y.Z
   ```

   That folds every fragment into a new `CHANGELOG.md` section, extends the
   link-reference chain and deletes the consumed fragments (see
   [`docs/changelog.d`](changelog.d/README.md)).
3. **Open a pull request** with the assembled changelog. CI runs on exactly the
   tree that will be tagged, and the generated section gets read before it is
   published. A fragment merged while the pull request is open belongs to the
   next release: merge `main` in, and reassemble only if the new fragment must
   ship now.
4. **After it merges, tag the merge commit** and push the tag:

   ```bash
   git tag -a vX.Y.Z -m "vX.Y.Z" <merge commit>
   git push origin vX.Y.Z
   ```

5. **The tag push creates the GitHub Release** (`.github/workflows/release.yml`),
   with that version's `CHANGELOG.md` section as its notes, marked Latest. The
   workflow refuses a tag whose section is missing or empty, which is what a tag
   pushed without its release pull request looks like.

## Data releases

The integrated-starlight maps (`starmap-*`) are published as GitHub Releases of
their own. Create them with `--latest=false`, so the Releases page goes on
showing the library's latest version rather than a data file.
