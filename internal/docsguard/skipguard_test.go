package docsguard_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// skipCall matches a call to Skip or Skipf on a *testing.T or testing.TB.
var skipCall = regexp.MustCompile(`^\s*\w+\.Skipf?\(`)

// errGuard matches `if err != nil {`, including the `if x, err := f(); err != nil {`
// form, and captures the name being tested so a variable that is not an error
// does not count.
var errGuard = regexp.MustCompile(`^\s*if (?:[\w, ]+ :?= [^;]+; )?(\w*[eE]rr) != nil \{\s*$`)

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
// # What counts as classified
//
// Any of [testutil.SkipOnUpstreamFailure], [testutil.Unreachable], an HTTP
// status test, or a package's own predicate such as spk.TransientHorizonsFault,
// appearing between the guard and the skip. The check is deliberately shallow:
// it asks whether the error was interrogated at all, not whether the answer was
// right. A shallow check that cannot be argued with is worth more here than a
// clever one that gets waived.
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
	root := filepath.Join("..", "..")

	classifiers := []string{
		"SkipOnUpstreamFailure",
		"Unreachable(",
		"StatusCode",
		"Transient",
		"errors.Is(",
		"errors.As(",
	}

	var checked, offenders int

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

		slash := filepath.ToSlash(rel)

		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil //nolint:nilerr // a file we cannot read is skipped, not fatal
		}

		checked++

		lines := strings.Split(string(src), "\n")

		for i, line := range lines {
			if !skipCall.MatchString(line) {
				continue
			}

			// Walk back over blank and comment lines to whatever opened this
			// block. A skip that is not the first statement inside an error
			// guard has something between it and the guard, which is the
			// classification this test is looking for.
			j := i - 1
			for j >= 0 && (strings.TrimSpace(lines[j]) == "" ||
				strings.HasPrefix(strings.TrimSpace(lines[j]), "//")) {
				j--
			}

			if j < 0 || !errGuard.MatchString(lines[j]) {
				continue
			}

			if mentionsAny(lines[j:i], classifiers) {
				continue
			}

			offenders++

			t.Errorf("%s:%d: skips on an error it has not identified.\n"+
				"  %s\n"+
				"  A test that skips on any error cannot fail. Ask what the error is:\n"+
				"  testutil.SkipOnUpstreamFailure(t, err) for a service having a bad day,\n"+
				"  testutil.Unreachable(err) for a request the network never carried, and\n"+
				"  t.Fatal for everything else — which is the part astrogo can fix.\n"+
				"  An environment precondition is a fine reason to skip; name the\n"+
				"  condition it means, rather than skipping on any error from the call.",
				slash, i+1, strings.TrimSpace(line))
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if checked == 0 {
		t.Fatal("no test files examined; the walk root may be wrong")
	}

	t.Logf("%d test files checked, %d unclassified skips", checked, offenders)
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
