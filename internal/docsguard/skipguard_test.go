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

// allowedSkip is a skip on a bare error that is genuinely about the machine the
// test is running on rather than about anything astrogo did.
type allowedSkip struct {
	file    string // repository-relative, forward slashes
	message string // a distinctive substring of the skip message
	why     string // not read by the test; here because the exemption has to argue itself
}

// The whole allowlist. Each entry is an environment precondition: something the
// test needs that has nothing to do with the code under test and that no
// classifier could sensibly be taught.
//
// Matched on the message rather than on a line number, so the exemption is tied
// to the thing being exempted and survives the file moving around it.
var allowedSkips = []allowedSkip{{
	file:    "atmosphere/dataset/cams/ground_truth_test.go",
	message: "real CAMS file not present",
	why:     "a licensed file staged by hand; absent on every machine but one",
}, {
	file:    "catalog/gaia/gaia_network_test.go",
	message: "is not resolvable",
	why:     "remote.URL refuses a disabled endpoint or an offline process, both deliberate",
}, {
	file:    "catalog/gaia/gaia_validation_test.go",
	message: "is not resolvable",
	why:     "as above",
}, {
	file:    "internal/testutil/unreachable_test.go",
	message: "cannot listen",
	why:     "the test needs to open a listener; a sandbox that forbids it is not a defect here",
}, {
	file:    "remote/unreachable_chain_test.go",
	message: "cannot reserve a local port",
	why:     "as above",
}, {
	file:    "remote/file/localfs_test.go",
	message: "cannot create a symlink here",
	why:     "Windows needs a privilege for symlinks that an ordinary account does not have",
}}

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
// # What is exempt
//
// An environment precondition — see allowedSkips, where each entry says what it
// is and why no classifier applies. Those are about the machine, not the code.
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

			if allowed(slash, line) || mentionsAny(lines[j:i], classifiers) {
				continue
			}

			offenders++

			t.Errorf("%s:%d: skips on an error it has not identified.\n"+
				"  %s\n"+
				"  A test that skips on any error cannot fail. Ask what the error is:\n"+
				"  testutil.SkipOnUpstreamFailure(t, err) for a service having a bad day,\n"+
				"  testutil.Unreachable(err) for a request the network never carried, and\n"+
				"  t.Fatal for everything else — which is the part astrogo can fix.\n"+
				"  An environment precondition goes in allowedSkips with its reason.",
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

// allowed reports whether this skip is one of the documented environment
// preconditions.
func allowed(file, line string) bool {
	for _, a := range allowedSkips {
		if a.file == file && strings.Contains(line, a.message) {
			return true
		}
	}

	return false
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
