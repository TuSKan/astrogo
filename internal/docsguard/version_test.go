package docsguard_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// docs are the files a reader meets first, relative to the repository root.
var docs = []string{
	"README.md",
	filepath.Join("docs", "ROADMAP.md"),
	filepath.Join("docs", "VALIDATION.md"),
	filepath.Join("docs", "skybrightness.md"),
}

// currentVersionClaim matches prose asserting which version the reader is
// looking at: "currently v0.5.0", "as of v0.5.0", "this release (v0.5.0)".
//
// Deliberately narrow. A document may say "shipped in v0.1.0" or "the v0.1.3
// release added topocentric corrections" as much as it likes — those are
// historical facts that stay true. What rots is a claim about *now*.
var currentVersionClaim = regexp.MustCompile(
	`(?i)(currently|as of|this release is|current version is)[^.\n]{0,20}v\d+\.\d+\.\d+`)

// TestDocsMakeNoCurrentVersionClaim keeps the front page from advertising a
// version nobody will remember to update.
//
// README.md said "astrogo is pre-1.0 (currently **v0.5.0**)" and went on
// describing that release's contents. It stayed there through ten minor
// releases, so a reader arriving at v0.15.1 was told the library was five
// versions younger and given a summary of changes long superseded. The
// "Known Limitations" section below it was pinned to v0.5.0 too.
//
// The cost is not the wrong number. It is that a reader who spots one stale
// claim has no way to tell which of the accuracy figures beside it is also
// stale — and this repository's whole argument is that its numbers can be
// trusted.
//
// The fix is to name no version: point at the tags and the changelog, which
// are generated from the thing itself and cannot drift.
func TestDocsMakeNoCurrentVersionClaim(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)

	for _, rel := range docs {
		t.Run(rel, func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(filepath.Join(root, rel))
			if err != nil {
				t.Fatalf("reading %s: %v", rel, err)
			}

			for i, line := range strings.Split(string(data), "\n") {
				if m := currentVersionClaim.FindString(line); m != "" {
					t.Errorf("%s:%d claims a current version: %q\n"+
						"  A version named in prose has to be updated by hand every release, and "+
						"is not.\n"+
						"  Say \"pre-1.0\" and link the releases page or CHANGELOG.md instead; a "+
						"historical\n"+
						"  statement (\"shipped in v0.1.0\") is fine and this check does not match it.",
						rel, i+1, m)
				}
			}
		})
	}
}

// repoRoot walks up from the test's directory until it finds go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	for range 10 {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}

		dir = parent
	}

	t.Fatal("could not find go.mod above the test's working directory")

	return ""
}
