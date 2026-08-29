package changelog

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

//nolint:gochecknoglobals // test flags must be package-level to register
var (
	update  = flag.Bool("update", false, "assemble changelog.d into CHANGELOG.md and delete the consumed fragments")
	version = flag.String("release-version", "", "version to assemble under, e.g. 0.17.0")
)

const (
	changelogPath = "../../CHANGELOG.md"
	fragmentDir   = "../../changelog.d"
	unreleased    = "## [Unreleased]"
)

// TestAssembleRelease folds changelog.d into CHANGELOG.md.
//
// Gated behind -update like the Horizons corpus generator and the accuracy
// table, so running the test suite never rewrites a checked-in file as a
// side effect. Without the flag it reports what a release would contain and
// writes nothing, which is also a useful thing to be able to ask.
func TestAssembleRelease(t *testing.T) {
	entries, err := LoadEntries(os.DirFS(fragmentDir))
	if err != nil {
		t.Fatalf("load fragments: %v", err)
	}

	if !*update {
		t.Logf("%d pending entries; rerun with -update -release-version X.Y.Z to assemble", len(entries))

		for _, e := range entries {
			t.Logf("  %-20s %s", e.Type, firstLine(e.Body))
		}

		return
	}

	if *version == "" {
		t.Fatal("-update requires -release-version, e.g. -release-version 0.17.0")
	}

	if len(entries) == 0 {
		t.Fatal("no fragments in changelog.d: nothing to release")
	}

	raw, err := os.ReadFile(changelogPath)
	if err != nil {
		t.Fatalf("read CHANGELOG.md: %v", err)
	}

	next, err := Assemble(string(raw), *version, today(), entries)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}

	if werr := os.WriteFile(changelogPath, []byte(next), 0o600); werr != nil {
		t.Fatalf("write CHANGELOG.md: %v", werr)
	}

	// Only once the new CHANGELOG is safely on disk: a fragment deleted
	// before that is an entry lost to a failed write.
	for _, e := range entries {
		if rerr := os.Remove(fragmentDir + "/" + e.Name); rerr != nil {
			t.Errorf("remove consumed fragment %s: %v", e.Name, rerr)
		}
	}

	t.Logf("assembled %d entries into %s and cleared changelog.d", len(entries), *version)
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")

	return line
}

func today() string { return time.Now().UTC().Format("2006-01-02") }

func TestAssembleInsertsSectionAndLink(t *testing.T) {
	src := "# Changelog\n\n" + unreleased + "\n\n" +
		"## [0.16.0] — 2026-08-29\n\n### Fixed\n- old thing\n\n" +
		"[Unreleased]: https://github.com/TuSKan/astrogo/compare/v0.16.0...HEAD\n" +
		"[0.16.0]: https://github.com/TuSKan/astrogo/compare/v0.15.1...v0.16.0\n"

	got, err := Assemble(src, "0.17.0", "2026-09-01", []Entry{
		{Type: "Fixed", PR: 61, Body: "**A fix.**"},
	})
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	for _, want := range []string{
		"## [0.17.0] — 2026-09-01",
		"### Fixed\n- **A fix.**",
		"[Unreleased]: https://github.com/TuSKan/astrogo/compare/v0.17.0...HEAD",
		"[0.17.0]: https://github.com/TuSKan/astrogo/compare/v0.16.0...v0.17.0",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("assembled CHANGELOG missing %q", want)
		}
	}

	// The previous release and its link must survive untouched — released
	// entries are never rewritten.
	if !strings.Contains(got, "## [0.16.0] — 2026-08-29") ||
		!strings.Contains(got, "[0.16.0]: https://github.com/TuSKan/astrogo/compare/v0.15.1...v0.16.0") {
		t.Error("assembling disturbed the previous release")
	}

	// An empty [Unreleased] must remain, ready for the next cycle.
	if !strings.Contains(got, unreleased) {
		t.Error("assembled CHANGELOG has no [Unreleased] heading left")
	}
}

func TestAssembleRefusesADuplicateVersion(t *testing.T) {
	src := "# Changelog\n\n" + unreleased + "\n\n## [0.16.0] — 2026-08-29\n\n" +
		"[Unreleased]: https://github.com/TuSKan/astrogo/compare/v0.16.0...HEAD\n"

	_, err := Assemble(src, "0.16.0", "2026-09-01", []Entry{{Type: "Fixed", Body: "x"}})
	if err == nil {
		t.Fatal("assembling over an existing version was allowed")
	}
}

func TestAssembleRefusesAChangelogWithoutUnreleased(t *testing.T) {
	_, err := Assemble("# Changelog\n\n## [0.16.0] — 2026-08-29\n", "0.17.0", "2026-09-01",
		[]Entry{{Type: "Fixed", Body: "x"}})
	if err == nil {
		t.Fatal("assembling into a CHANGELOG with no [Unreleased] was allowed")
	}
}

// TestAssembleIsDeterministic matters because the output is committed: the
// same inputs must not produce a diff.
func TestAssembleIsDeterministic(t *testing.T) {
	src := "# Changelog\n\n" + unreleased + "\n\n## [0.16.0] — 2026-08-29\n\n" +
		"[Unreleased]: https://github.com/TuSKan/astrogo/compare/v0.16.0...HEAD\n"

	entries := []Entry{
		{Type: "Fixed", PR: 2, Body: "**b**"},
		{Type: "Added", PR: 1, Body: "**a**"},
		{Type: "Fixed", PR: 3, Body: "**c**"},
	}

	first, err := Assemble(src, "0.17.0", "2026-09-01", entries)
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	for range 5 {
		again, aerr := Assemble(src, "0.17.0", "2026-09-01", entries)
		if aerr != nil {
			t.Fatalf("Assemble: %v", aerr)
		}

		if again != first {
			t.Fatal("Assemble is not deterministic across runs")
		}
	}
}

// TestFragmentsSurviveAFailedWrite is the ordering guarantee spelled out:
// LoadEntries must not mutate the directory, so a later failure loses
// nothing.
func TestFragmentsSurviveAFailedWrite(t *testing.T) {
	dir := fstest.MapFS{
		"1-a.md": {Data: []byte("---\ntype: Fixed\npr: 1\n---\nbody\n")},
	}

	if _, err := LoadEntries(dir); err != nil {
		t.Fatalf("LoadEntries: %v", err)
	}

	if _, err := dir.Open("1-a.md"); err != nil {
		t.Errorf("LoadEntries consumed a fragment: %v", err)
	}
}

func ExampleRender() {
	fmt.Print(Render([]Entry{
		{Type: "Fixed", PR: 2, Body: "**Second.**"},
		{Type: "Added", PR: 1, Body: "**First.**"},
	}))
	// Output:
	//
	// ### Added
	// - **First.**
	//
	// ### Fixed
	// - **Second.**
}

// TestAssembleMergesHandWrittenUnreleasedEntries is the case the end-to-end
// trial caught: the file had entries written directly under [Unreleased]
// before changelog.d existed, and appending the fragments below them
// produced two "### Fixed" headings in one release — precisely the silent
// mis-filing this package exists to prevent.
func TestAssembleMergesHandWrittenUnreleasedEntries(t *testing.T) {
	src := "# Changelog\n\n" + unreleased + "\n\n### Fixed\n- **Written by hand.**\n\n" +
		"### Added\n- **Also by hand.**\n\n" +
		"## [0.16.0] — 2026-08-29\n\n### Fixed\n- released, untouched\n\n" +
		"[Unreleased]: https://github.com/TuSKan/astrogo/compare/v0.16.0...HEAD\n"

	got, err := Assemble(src, "0.17.0", "2026-09-01", []Entry{
		{Type: "Fixed", PR: 61, Body: "**From a fragment.**"},
	})
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	release, _, _ := strings.Cut(strings.SplitN(got, "## [0.17.0]", 2)[1], "## [0.16.0]")

	if n := strings.Count(release, "### Fixed"); n != 1 {
		t.Errorf("release has %d Fixed headings, want 1:\n%s", n, release)
	}

	for _, want := range []string{"**Written by hand.**", "**From a fragment.**", "**Also by hand.**"} {
		if !strings.Contains(release, want) {
			t.Errorf("release lost entry %q", want)
		}
	}

	// Canonical order survives the merge.
	if strings.Index(release, "### Added") > strings.Index(release, "### Fixed") {
		t.Error("sections are not in canonical order after merging")
	}
}

// TestAssembleKeepsAnUnknownSection: a heading this package does not know
// is still somebody's entry. Filing it last beats dropping it.
func TestAssembleKeepsAnUnknownSection(t *testing.T) {
	src := "# Changelog\n\n" + unreleased + "\n\n### Notes\n- **Something odd.**\n\n" +
		"## [0.16.0] — 2026-08-29\n\n" +
		"[Unreleased]: https://github.com/TuSKan/astrogo/compare/v0.16.0...HEAD\n"

	got, err := Assemble(src, "0.17.0", "2026-09-01", []Entry{{Type: "Fixed", Body: "**x**"}})
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	if !strings.Contains(got, "**Something odd.**") {
		t.Error("an unrecognised section was dropped")
	}
}
