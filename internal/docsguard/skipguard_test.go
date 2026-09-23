package docsguard_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// skipCall matches a statement that ends a test by skipping it: Skip or Skipf
// on a *testing.T or testing.TB, or metrology.NotVerified, which records NOT
// VERIFIED in the accuracy report and then calls Skipf itself.
//
// The indentation is captured because the enclosing block is found by it.
var skipCall = regexp.MustCompile(`^(\t*)(?:\w+\.Skipf?|metrology\.NotVerified)\(`)

// errGuard matches a block that opens on an unexamined error: `if err != nil {`,
// the `if x, err := f(); err != nil {` form, and either behind `} else `. The
// name is captured so a variable that is not an error does not count.
var errGuard = regexp.MustCompile(`^\t*(?:\} else )?if (?:[\w, ]+ :?= [^;]+; )?(\w*[eE]rr) != nil \{\s*$`)

// funcDecl matches a top-level function declaration and captures its name.
var funcDecl = regexp.MustCompile(`^func (?:\([^)]*\) )?(\w+)\(`)

// errorParam matches a parameter of type error in a signature line.
var errorParam = regexp.MustCompile(`\w+ error[,)]`)

// classifiers are the ways a helper can identify an error before skipping on
// it. Consulted for helpers only; a call site is judged by structure alone.
var classifiers = []string{
	"SkipOnUpstreamFailure",
	"UpstreamFailure(",
	"Unreachable(",
	"StatusCode",
	"Transient",
	"errors.Is(",
	"errors.As(",
}

// TestSkipsOnAnErrorAreClassified is the rule that a test may not skip on an
// error it has not identified.
//
// # Why this is a rule
//
// A test that skips on any error cannot fail. It reads green forever, including
// when the thing it covers is permanently broken, and unlike a failing test
// nobody ever looks at it. The repository had 45 of these, and the skip messages
// are what gave them away: "CAMS did not answer", "SkyCalc did not answer",
// "IMCCE JSON decode error", one that said "(network issue?)" out loud and one
// that asserted "(external, not astrogo)" in prose while the code around it
// could not tell. Every one of those names a cause the code never established.
//
// The worst of them skipped on a json.Decode of a document into a struct
// declared in this repository — a schema change or a wrong field would have
// skipped, quietly, for as long as anyone cared to look.
//
// # The rule, stated structurally
//
// A skip whose immediately enclosing block opens on a bare `if err != nil` is a
// skip on any error that reaches it, wherever in the block it sits. A skip that
// is meant is nested inside the branch that identified the error:
//
//	if err != nil {
//		if errors.Is(err, fs.ErrNotExist) {
//			t.Skipf("fixture not present")   // enclosed by the identifying branch
//		}
//		testutil.SkipOnUpstreamFailure(t, err)
//		t.Fatalf("...: %v", err)
//	}
//
// The enclosing block is found by indentation, which gofmt makes exact.
//
// # Two holes the first version had
//
// It only looked at a skip that was the *first* statement after the guard, and
// only at a skip spelled Skip or Skipf. A sweep found both open:
//
//   - A skip left after a classifier — `if spk.TransientHorizonsFault(err) {
//     t.Skipf(...) }` followed by an unconditional `t.Skipf` for everything else —
//     passed, because the line before the second skip was a closing brace. That
//     second skip fires on every error the first one declined.
//   - metrology.NotVerified calls Skipf after recording its result. Four suites
//     called it on any provider error, so a regression that broke
//     jpl.NewProvider outright was reported as NAIF being down, and those suites
//     could not fail. The guard never saw a Skip.
//
// Both are closed here, and so is the general form of the second: a helper in
// the same package that skips at its top level is treated as a skip wherever it
// is called.
//
// # Helpers that are handed an error
//
// A function that takes an error and skips at its top level without looking at
// it is the same defect with a name on it, reached from every call site at once.
// vizier's skipOnServerUnavailable was one: it skipped on whatever survived its
// retries, which included the "unresolved identifiers" 400 that a genuinely
// broken query also returns. A helper must identify the error before it skips,
// by the same predicates a call site would use.
//
// # There is no exemption list
//
// There was one, briefly, holding eight environment preconditions: a licensed
// file staged by hand, an endpoint switched off, a listener, a port, a Windows
// symlink privilege. Every one of them turned out to have a specific condition
// it meant and was skipping on any error from the same call, which is this
// defect one level down — so they were classified instead and the list went
// away.
//
// One of them is the argument for not having a list. The symlink skips were
// exempt because "Windows needs a privilege" is obviously an environment
// matter. It is, and the obvious predicate for it does not work:
// ERROR_PRIVILEGE_NOT_HELD does not satisfy errors.Is(err, fs.ErrPermission),
// which the exemption would have gone on hiding.
//
// An environment precondition is still a fine reason to skip. It just has to
// say which one, like everything else.
func TestSkipsOnAnErrorAreClassified(t *testing.T) {
	files := readTestFiles(t, filepath.Join("..", "..", ""))
	skippers := skippingHelpers(files)

	var offenders int

	for path, lines := range files {
		local := skippers[filepath.Dir(path)]

		for i, line := range lines {
			if !isSkip(line, local) {
				continue
			}

			j := enclosingOpener(lines, i)
			if j < 0 || !errGuard.MatchString(lines[j]) {
				continue
			}

			offenders++

			t.Errorf("%s:%d: skips on an error it has not identified.\n"+
				"  %s\n"+
				"  inside: %s\n"+
				"  A test that skips on any error cannot fail. Ask what the error is, and\n"+
				"  skip inside the branch that answered: testutil.SkipOnUpstreamFailure for\n"+
				"  a service having a bad day, errors.Is for a named precondition, and\n"+
				"  t.Fatal for everything else — which is the part astrogo can fix.",
				path, i+1, strings.TrimSpace(line), strings.TrimSpace(lines[j]))
		}

		for _, h := range unclassifiedErrorHelpers(lines) {
			offenders++

			t.Errorf("%s:%d: %s is handed an error and skips without identifying it.\n"+
				"  Every call site inherits that: whatever error reaches it becomes a pass.\n"+
				"  Classify first — testutil.SkipOnUpstreamFailure, errors.Is — and fail\n"+
				"  on the rest.",
				path, h.line, h.name)
		}
	}

	if len(files) == 0 {
		t.Fatal("no test files examined; the walk root may be wrong")
	}

	t.Logf("%d test files checked, %d unclassified skips", len(files), offenders)
}

// readTestFiles returns every _test.go file under root, split into lines and
// keyed by repository-relative, forward-slashed path.
func readTestFiles(t *testing.T, root string) map[string][]string {
	t.Helper()

	out := map[string][]string{}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable path is skipped, not fatal
		}

		if info.IsDir() {
			if name := info.Name(); name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}

		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil //nolint:nilerr // an unrelatable path is skipped, not fatal
		}

		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil //nolint:nilerr // a file we cannot read is skipped, not fatal
		}

		out[filepath.ToSlash(rel)] = strings.Split(string(src), "\n")

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	return out
}

// isSkip reports whether line ends a test by skipping it, directly or through a
// helper in the same package that does.
func isSkip(line string, local map[string]bool) bool {
	if skipCall.MatchString(line) {
		return true
	}

	trimmed := strings.TrimSpace(line)

	for name := range local {
		if strings.HasPrefix(trimmed, name+"(") {
			return true
		}
	}

	return false
}

// skippingHelpers finds, per package directory, the functions that skip at
// their top level — so that calling one is, to its caller, a skip.
func skippingHelpers(files map[string][]string) map[string]map[string]bool {
	out := map[string]map[string]bool{}

	for path, lines := range files {
		dir := filepath.Dir(path)

		for _, f := range functions(lines) {
			if !f.skipsAtTopLevel(lines) {
				continue
			}

			if out[dir] == nil {
				out[dir] = map[string]bool{}
			}

			out[dir][f.name] = true
		}
	}

	return out
}

// helperFinding is one helper that skips on an error it never looked at.
type helperFinding struct {
	name string
	line int
}

// unclassifiedErrorHelpers returns the functions in lines that take an error
// and skip at their top level with no classifier before the skip.
func unclassifiedErrorHelpers(lines []string) []helperFinding {
	var out []helperFinding

	for _, f := range functions(lines) {
		if !errorParam.MatchString(lines[f.start]) {
			continue
		}

		for i := f.start + 1; i < f.end; i++ {
			if !skipCall.MatchString(lines[i]) || indentOf(lines[i]) != 1 {
				continue
			}

			if !mentionsAny(lines[f.start:i], classifiers) {
				out = append(out, helperFinding{name: f.name, line: i + 1})
			}
		}
	}

	return out
}

// function is a top-level declaration: its name and the lines of its body.
type function struct {
	name       string
	start, end int
}

// skipsAtTopLevel reports whether the function's body skips outside any nested
// block — that is, whether a call to it can end the caller's test on its own.
func (f function) skipsAtTopLevel(lines []string) bool {
	for i := f.start + 1; i < f.end; i++ {
		if skipCall.MatchString(lines[i]) && indentOf(lines[i]) == 1 {
			return true
		}
	}

	return false
}

// functions lists the top-level function declarations in lines. A body ends at
// the first line that is exactly "}", which gofmt guarantees.
func functions(lines []string) []function {
	var out []function

	for i := 0; i < len(lines); i++ {
		m := funcDecl.FindStringSubmatch(lines[i])
		if m == nil || strings.HasSuffix(strings.TrimSpace(lines[i]), "}") {
			continue
		}

		end := i + 1
		for end < len(lines) && lines[end] != "}" {
			end++
		}

		out = append(out, function{name: m[1], start: i, end: end})
		i = end
	}

	return out
}

// enclosingOpener returns the index of the line that opens the block holding
// lines[i], or -1. In gofmt-formatted Go a statement sits exactly one tab deeper
// than the line opening its block, and nothing between them sits at the
// opener's depth — so the first shallower line walking back is the opener.
func enclosingOpener(lines []string, i int) int {
	depth := indentOf(lines[i])

	for j := i - 1; j >= 0; j-- {
		s := strings.TrimSpace(lines[j])
		if s == "" || strings.HasPrefix(s, "//") {
			continue
		}

		switch d := indentOf(lines[j]); {
		case d == depth-1 && strings.HasSuffix(s, "{"):
			return j
		case d < depth-1:
			return -1
		}
	}

	return -1
}

// indentOf counts the leading tabs on line.
func indentOf(line string) int {
	return len(line) - len(strings.TrimLeft(line, "\t"))
}

// mentionsAny reports whether any of the needles appears in the block.
func mentionsAny(block []string, needles []string) bool {
	joined := strings.Join(block, "\n")

	for _, n := range needles {
		if strings.Contains(joined, n) {
			return true
		}
	}

	return false
}
