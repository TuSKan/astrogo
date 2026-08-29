package changelog

import (
	"errors"
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

func TestParseEntry(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		wantErr error
		want    Entry
	}{
		{
			name: "well formed",
			src:  "---\ntype: Fixed\npr: 61\n---\n**A thing.** Why it mattered.\n",
			want: Entry{Type: "Fixed", PR: 61, Body: "**A thing.** Why it mattered."},
		},
		{
			name: "multi-line body",
			src:  "---\ntype: Added\npr: 7\n---\nfirst line\nsecond line\n",
			want: Entry{Type: "Added", PR: 7, Body: "first line\nsecond line"},
		},
		{
			name: "the BREAKING variant is a real section here",
			src:  "---\ntype: Changed — BREAKING\npr: 1\n---\nbody\n",
			want: Entry{Type: "Changed — BREAKING", PR: 1, Body: "body"},
		},
		{
			name: "no pr number is allowed",
			src:  "---\ntype: Security\n---\nbody\n",
			want: Entry{Type: "Security", PR: 0, Body: "body"},
		},

		// Each of these fails the build rather than surfacing at release
		// time, which is the whole reason this parser has a test.
		{name: "no front matter", src: "type: Fixed\nbody\n", wantErr: ErrNoFrontMatter},
		{name: "unterminated front matter", src: "---\ntype: Fixed\nbody\n", wantErr: ErrNoFrontMatter},
		{name: "no type", src: "---\npr: 3\n---\nbody\n", wantErr: ErrNoType},
		{name: "misspelled type", src: "---\ntype: Fixes\n---\nbody\n", wantErr: ErrUnknownType},
		{name: "wrong case", src: "---\ntype: fixed\n---\nbody\n", wantErr: ErrUnknownType},
		{name: "empty body", src: "---\ntype: Fixed\n---\n\n", wantErr: ErrEmptyBody},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseEntry(c.name+".md", c.src)

			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("ParseEntry = %v, want %v", err, c.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("ParseEntry: %v", err)
			}

			if got.Type != c.want.Type || got.PR != c.want.PR || got.Body != c.want.Body {
				t.Errorf("ParseEntry = %+v, want type=%q pr=%d body=%q",
					got, c.want.Type, c.want.PR, c.want.Body)
			}
		})
	}
}

// TestParseEntryNamesTheFile is not cosmetic: a release assembles every
// fragment at once, and an error that does not say which file is wrong
// sends the reader through all of them.
func TestParseEntryNamesTheFile(t *testing.T) {
	_, err := ParseEntry("0061-some-fix.md", "---\ntype: Nonsense\n---\nbody\n")
	if err == nil {
		t.Fatal("expected an error")
	}

	if !strings.Contains(err.Error(), "0061-some-fix.md") {
		t.Errorf("error does not name the offending file: %v", err)
	}
}

func TestLoadEntriesSkipsReadme(t *testing.T) {
	dir := fstest.MapFS{
		"README.md":   {Data: []byte("# docs/changelog.d\n\nNot an entry.\n")},
		"58-thing.md": {Data: []byte("---\ntype: Added\npr: 58\n---\nbody\n")},
		"59-other.md": {Data: []byte("---\ntype: Fixed\npr: 59\n---\nbody\n")},
		"notes.txt":   {Data: []byte("ignored, not markdown")},
		"60-third.md": {Data: []byte("---\ntype: Fixed\npr: 60\n---\nbody\n")},
	}

	got, err := LoadEntries(dir)
	if err != nil {
		t.Fatalf("LoadEntries: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("loaded %d entries, want 3 (README.md and notes.txt excluded)", len(got))
	}
}

// TestLoadEntriesReportsEveryBadFragment is the difference between one fix
// and one fix per iteration.
func TestLoadEntriesReportsEveryBadFragment(t *testing.T) {
	dir := fstest.MapFS{
		"1-bad.md":  {Data: []byte("no front matter\n")},
		"2-bad.md":  {Data: []byte("---\ntype: Nope\n---\nbody\n")},
		"3-good.md": {Data: []byte("---\ntype: Fixed\npr: 3\n---\nbody\n")},
	}

	_, err := LoadEntries(dir)
	if err == nil {
		t.Fatal("expected an error")
	}

	for _, want := range []string{"1-bad.md", "2-bad.md"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %s: %v", want, err)
		}
	}
}

func TestRenderOrdersSectionsAndEntries(t *testing.T) {
	got := Render([]Entry{
		{Type: "Fixed", PR: 20, Body: "**Second fix.**"},
		{Type: "Added", PR: 9, Body: "**An addition.**"},
		{Type: "Fixed", PR: 10, Body: "**First fix.**"},
		{Type: "Changed — BREAKING", PR: 1, Body: "**A break.**"},
	})

	want := "\n### Added\n- **An addition.**\n" +
		"\n### Changed — BREAKING\n- **A break.**\n" +
		"\n### Fixed\n- **First fix.**\n- **Second fix.**\n"

	if got != want {
		t.Errorf("Render =\n%q\nwant\n%q", got, want)
	}
}

// TestRenderEmpty distinguishes "nothing to release" from "a release with
// no notes", which the caller has to be able to tell apart.
func TestRenderEmpty(t *testing.T) {
	if got := Render(nil); got != "" {
		t.Errorf("Render(nil) = %q, want empty", got)
	}
}

// TestCheckedInFragmentsParse runs in ordinary CI, so a malformed fragment
// fails the pull request that added it rather than the release weeks later.
func TestCheckedInFragmentsParse(t *testing.T) {
	dir := os.DirFS("../../docs/changelog.d")

	entries, err := LoadEntries(dir)
	if err != nil {
		t.Fatalf("docs/changelog.d contains a malformed fragment: %v", err)
	}

	t.Logf("%d pending changelog entries", len(entries))
}
