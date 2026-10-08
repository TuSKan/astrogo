package changelog

import (
	"errors"
	"flag"
	"os"
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

		if _, err := ReleaseNotes(string(src), newest[1]); err != nil {
			t.Errorf("the newest release, %s: %v", newest[1], err)
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
