package changelog

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"
)

var (
	notesVersion = flag.String("release-notes", "", "write this version's CHANGELOG.md section to -notes-out, e.g. 0.20.0")
	notesOut     = flag.String("notes-out", "", "where -release-notes writes the notes")
)

const notesFixture = `# Changelog

## [Unreleased]

## [0.2.0] — 2026-10-12

### Fixed

- Two. [#2]

## [0.1.0] — 2026-10-05

### Added

- One. [#1]

## [0.0.9] — 2026-10-01

[Unreleased]: https://example.org/compare/v0.2.0...HEAD
[0.2.0]: https://example.org/compare/v0.1.0...v0.2.0
`

// TestReleaseNotes reads a section between two headings, the oldest section
// up to the link references, and refuses a version with no section or an
// empty one.
func TestReleaseNotes(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		version, want string
		err           error
	}{
		{"0.2.0", "### Fixed\n\n- Two. [#2]", nil},
		{"0.1.0", "### Added\n\n- One. [#1]", nil},
		{"0.0.9", "", ErrEmptyRelease},
		{"0.3.0", "", ErrNoSuchRelease},
		{"0.2", "", ErrNoSuchRelease},
	} {
		got, err := ReleaseNotes(notesFixture, c.version)
		if !errors.Is(err, c.err) {
			t.Errorf("ReleaseNotes(%s) error = %v, want %v", c.version, err, c.err)
		}

		if got != c.want {
			t.Errorf("ReleaseNotes(%s) = %q, want %q", c.version, got, c.want)
		}
	}
}

// TestReleaseNotesFile is what the release workflow runs on a tag push: with
// -release-notes and -notes-out it writes that version's section of the real
// CHANGELOG.md to a file for the GitHub Release. Without them it checks that
// the newest released section of the real file reads as notes, so a changed
// heading format fails ordinary CI rather than the next release.
func TestReleaseNotesFile(t *testing.T) {
	src, err := os.ReadFile(changelogPath)
	if err != nil {
		t.Fatalf("read %s: %v", changelogPath, err)
	}

	if *notesVersion == "" {
		newest := versionHeading.FindStringSubmatch(string(src))
		if newest == nil {
			t.Fatalf("%s has no released section", changelogPath)
		}

		notes, err := ReleaseNotes(string(src), newest[1])
		if err != nil {
			t.Errorf("the newest release, %s: %v", newest[1], err)
		}

		if len(notes) > githubReleaseBodyLimit {
			t.Errorf("the newest release's notes, %s, are %d bytes, over GitHub's %d",
				newest[1], len(notes), githubReleaseBodyLimit)
		}

		return
	}

	if *notesOut == "" {
		t.Fatal("-release-notes needs -notes-out")
	}

	notes, err := ReleaseNotes(string(src), *notesVersion)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(*notesOut, []byte(notes+"\n"), 0o600); err != nil {
		t.Fatalf("write %s: %v", *notesOut, err)
	}
}

// TestReleaseNotesFitAGitHubRelease is v0.20.0's release: its section came
// to 165,662 characters, GitHub refuses a release body over 125,000, and the
// tag's release failed at creation. Notes that fit are returned unchanged;
// notes that do not keep whole sections in priority order, in their own
// order, and point to CHANGELOG.md at the tag for the rest.
func TestReleaseNotesFitAGitHubRelease(t *testing.T) {
	t.Parallel()

	entries := func(prefix string, size int) string {
		var b strings.Builder

		for i := 0; b.Len() < size; i++ {
			fmt.Fprintf(&b, "- %s entry %d, long enough to take some room in the notes. (#%d)\n", prefix, i, i)
		}

		return strings.TrimSpace(b.String())
	}

	release := func(sections ...string) string {
		return "# Changelog\n\n## [Unreleased]\n\n## [0.2.0] — 2026-10-12\n\n" +
			strings.Join(sections, "\n\n") + "\n\n## [0.1.0] — 2026-10-05\n\n### Added\n\n- One. (#1)\n\n" +
			"[Unreleased]: https://example.org/r/compare/v0.2.0...HEAD\n" +
			"[0.2.0]: https://example.org/r/compare/v0.1.0...v0.2.0\n"
	}

	const link = "[CHANGELOG.md](https://example.org/r/blob/v0.2.0/CHANGELOG.md)"

	added := entries("Added", 70_000)
	breaking := entries("Breaking", 10_000)
	fixed := entries("Fixed", 70_000)

	got, err := ReleaseNotes(release(
		"### Added\n\n"+added,
		"### Changed — BREAKING\n\n"+breaking,
		"### Fixed\n\n"+fixed,
	), "0.2.0")
	if err != nil {
		t.Fatal(err)
	}

	if len(got) > githubReleaseBodyLimit {
		t.Fatalf("notes are %d bytes, over %d", len(got), githubReleaseBodyLimit)
	}

	// Added outranks Fixed and only one of the two fits beside the breaking
	// changes, so Added and the breaking changes stay whole and Fixed points
	// to the changelog.
	for _, want := range []string{added, breaking, link} {
		if !strings.Contains(got, want) {
			t.Errorf("notes leave out %.60q", want)
		}
	}

	if strings.Contains(got, "Fixed entry 0,") {
		t.Errorf("notes kept Fixed, which does not fit beside Added")
	}

	if a, b, f := strings.Index(got, "### Added"), strings.Index(got, "### Changed — BREAKING"),
		strings.Index(got, "### Fixed"); a < 0 || b < a || f < b {
		t.Errorf("sections out of their own order: Added %d, Changed — BREAKING %d, Fixed %d", a, b, f)
	}

	// Notes that fit are the section as it is, with no pointer.
	small, err := ReleaseNotes(release("### Fixed\n\n- Two. (#2)"), "0.2.0")
	if err != nil || small != "### Fixed\n\n- Two. (#2)" {
		t.Errorf("notes that fit = %q, %v; want the section unchanged", small, err)
	}
}
