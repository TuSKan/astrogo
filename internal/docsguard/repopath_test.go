package docsguard_test

import (
	"bufio"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// repoFileRef matches a path to a Go or Markdown file named in prose, at the
// start of a line or after a character that can open a path: whitespace, a
// backtick, a bracket, a parenthesis, a quote. Requiring that opening keeps out
// a token inside a URL, a shell variable such as $RUNNER_TEMP, and a format
// string's %s. A path starting ./ or ../ may be a single file; any other needs
// a directory, since a bare file name could be anybody's.
var repoFileRef = regexp.MustCompile(
	"(?:^|[\\s`(\\[\"'<])(" +
		"(?:\\.{1,2}/)+[A-Za-z0-9_][A-Za-z0-9_.-]*(?:/[A-Za-z0-9_.-]+)*\\.(?:go|md)" +
		"|[A-Za-z0-9_][A-Za-z0-9_.-]*(?:/[A-Za-z0-9_.-]+)+\\.(?:go|md)" +
		")\\b")

// markdownLinkRef matches a Markdown link's target when it is a Go or Markdown
// file, with any #anchor after it.
var markdownLinkRef = regexp.MustCompile(`\]\(([^()\s#?]+\.(?:go|md))(?:#[^()\s]*)?\)`)

// TestNamedRepositoryFilesExist holds every repository file a comment or a
// document names to the tree (#1071).
//
// # Why nothing else catches this
//
// TestGoDocLinksResolve resolves symbols, and TestCitedSymbolsExist resolves
// the changelog's citations, but a file named in prose is checked by nothing.
// So when a file moves or is deleted, every comment that points a reader at it
// goes on pointing. When this test was written it found eight: a contributor
// guide naming the init it described at a path that file had left, a design
// doc naming the Vallado test by a name it no longer had, a comment saying a
// deleted file "does read it live", a test quoting a deleted file's doc
// comment, and four more.
//
// # What counts as a repository path
//
// In prose, a token ending in .go or .md whose first segment is a top-level
// entry of the repository, read from the root, or one starting with ./ or
// ../, read from the file's own directory. Anything else is someone else's
// path: gofa's own test files, a parent package's testdata named relative to
// it, a file inside a fixture. A token whose inner segments end in .go or .md
// names two files joined by a slash, not one path.
//
// A Markdown link's target is read from the linking file's directory, as
// GitHub resolves it, or from the root when it starts with a slash. A link
// is checked that way and not again as prose: a target under docs that
// repeats the docs directory is a broken link, even though the same words in
// prose name a file that exists.
//
// Go files are read for their comments only, since a string literal is data,
// not a pointer for a reader. Markdown is read whole, apart from CHANGELOG.md
// and docs/changelog.d, which record what was true when they were written. A
// deleted file is still fine to mention: say what it was without giving a
// path that no longer resolves.
func TestNamedRepositoryFilesExist(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read %s: %v", root, err)
	}

	tops := make(map[string]bool, len(entries))
	for _, e := range entries {
		tops[e.Name()] = true
	}

	var stale []string

	report := func(file string, line int, ref string) {
		rel, _ := filepath.Rel(root, file)
		stale = append(stale, filepath.ToSlash(rel)+":"+strconv.Itoa(line)+": "+ref)
	}

	check := func(file, text string, line int) {
		if strings.HasSuffix(file, ".md") {
			for _, m := range markdownLinkRef.FindAllStringSubmatch(text, -1) {
				ref := m[1]
				if strings.Contains(ref, ":") {
					continue // a URL, or a scheme such as mailto:
				}

				target := filepath.Join(filepath.Dir(file), filepath.FromSlash(ref))
				if strings.HasPrefix(ref, "/") {
					target = filepath.Join(root, filepath.FromSlash(ref))
				}

				if _, err := os.Stat(target); err != nil {
					report(file, line, ref)
				}
			}

			text = markdownLinkRef.ReplaceAllString(text, "]()")
		}

		for _, m := range repoFileRef.FindAllStringSubmatch(text, -1) {
			ref := m[1]
			segments := strings.Split(ref, "/")

			if twoFiles(segments) {
				continue
			}

			var target string

			switch {
			case segments[0] == "." || segments[0] == "..":
				target = filepath.Join(filepath.Dir(file), filepath.FromSlash(ref))
			case tops[segments[0]]:
				target = filepath.Join(root, filepath.FromSlash(ref))
			default:
				continue
			}

			if _, err := os.Stat(target); err != nil {
				report(file, line, ref)
			}
		}
	}

	err = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable path contributes nothing and is not fatal
		}

		rel := filepath.ToSlash(strings.TrimPrefix(p, root+string(filepath.Separator)))

		if info.IsDir() {
			switch info.Name() {
			case ".git", "node_modules", "testdata":
				return filepath.SkipDir
			}

			if rel == "docs/changelog.d" {
				return filepath.SkipDir
			}

			return nil
		}

		switch {
		case strings.HasSuffix(p, ".go"):
			goCommentRefs(t, p, check)
		case strings.HasSuffix(p, ".md") && rel != "CHANGELOG.md":
			markdownRefs(t, p, check)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}

	for _, s := range stale {
		t.Errorf("%s names a repository file that does not exist", s)
	}
}

// twoFiles reports whether a token's inner segments are themselves Go or
// Markdown files, as in "reader_test.go/api_test.go", which names two files.
func twoFiles(segments []string) bool {
	for _, s := range segments[:len(segments)-1] {
		if ext := path.Ext(s); ext == ".go" || ext == ".md" {
			return true
		}
	}

	return false
}

// goCommentRefs passes each line of each comment in a Go file to check.
func goCommentRefs(t *testing.T, file string, check func(file, text string, line int)) {
	t.Helper()

	fset := token.NewFileSet()

	f, err := parser.ParseFile(fset, file, nil, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		// Not this test's business: a file that does not parse fails the build.
		return
	}

	for _, group := range f.Comments {
		for _, c := range group.List {
			start := fset.Position(c.Pos()).Line
			for i, line := range strings.Split(c.Text, "\n") {
				check(file, line, start+i)
			}
		}
	}
}

// markdownRefs passes each line of a Markdown file to check.
func markdownRefs(t *testing.T, file string, check func(file, text string, line int)) {
	t.Helper()

	f, err := os.Open(file)
	if err != nil {
		t.Errorf("open %s: %v", file, err)

		return
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for n := 1; sc.Scan(); n++ {
		check(file, sc.Text(), n)
	}

	if err := sc.Err(); err != nil {
		t.Errorf("read %s: %v", file, err)
	}
}
