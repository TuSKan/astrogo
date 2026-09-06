package docsguard_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// externalTag matches the build tags that mark a test as talking to somebody
// else's service.
var externalTag = regexp.MustCompile(`(?m)^//go:build .*\b(network|integration)\b`)

// errCheckFailure matches `if <err> != nil {` followed immediately by a
// t.Fatal/t.Error call, capturing the error variable's name and the call line.
//
// Written against the shape rather than parsed, matching this package's other
// guards. Whether the error is actually reported is decided in Go below: RE2
// has no lookahead, and a format string may contain a parenthesis of its own —
// t.Fatalf("Var(lnsp): %v", err) — so a pattern that tried to stop at the
// closing parenthesis would misread the call and flag a test that does report.
var errCheckFailure = regexp.MustCompile(
	`(?m)^[ \t]*if ([a-zA-Z0-9_]*[eE]rr[a-zA-Z0-9_]*) != nil \{\n([ \t]*(?:t|b|tb)\.(?:Fatal|Fatalf|Error|Errorf)\([^\n]*)`)

// TestExternalTestsReportWhyTheyFailed enforces, on the tests that reach
// somebody else's service, that a failure says what went wrong.
//
// # Where the rule comes from
//
// CLAUDE.md states it plainly for the network tag: these tests skip when the
// endpoint is unreachable and keep t.Fatal for wrong data from a reachable
// one. internal/testutil.SkipOnUpstreamFailure is what draws that line — 429,
// 5xx, 408, deadline-exceeded and network timeouts are "not verified", not
// "wrong".
//
// # What this guard checks, and why it is the half worth checking
//
// Whether a test called SkipOnUpstreamFailure cannot be established from the
// source without knowing which error reaches which check. Whether it reported
// the error at all can. catalog/norad's TestResolve_Live failed a pull request
// that touched only fits with:
//
//	norad_network_test.go:144: Failed to resolve ISS
//
// and that was the entire diagnostic, because the error was discarded. The
// test one line below it resolved the ISS successfully 1.2 s later, so the
// service was reachable — but nothing in the output could establish that, and
// a throttle, an outage, and CelesTrak genuinely not knowing what the ISS is
// all produce that same sentence.
//
// This is the error-versus-absence discipline the library holds itself to
// (#102, #172, #177, #182), applied to its own tests: a failure that cannot be
// told apart from a different failure is the thing being ruled out, and
// carrying the error is what tells them apart. See #203.
func TestExternalTestsReportWhyTheyFailed(t *testing.T) {
	root := filepath.Join("..", "..")

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

		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil //nolint:nilerr // a file we cannot read is skipped, not fatal
		}

		body := string(src)
		if !externalTag.MatchString(body) {
			return nil
		}

		checked++

		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil //nolint:nilerr // an unrelatable path is skipped, not fatal
		}

		for _, m := range errCheckFailure.FindAllStringSubmatchIndex(body, -1) {
			name := body[m[2]:m[3]]
			call := strings.TrimSpace(body[m[4]:m[5]])

			// The error is reported. Nothing to say.
			if strings.Contains(call, name) {
				continue
			}

			// A call whose arguments run onto the next line cannot be read one
			// line at a time. Left alone deliberately: a guard that guessed
			// here would spend its credibility on false positives, and the
			// case it exists for is the one-liner.
			if strings.Count(call, "(") != strings.Count(call, ")") {
				continue
			}

			offenders++

			t.Errorf("%s:%d: fails on an error without reporting it.\n"+
				"  %s\n"+
				"  Pass %s into the message, and put\n"+
				"  testutil.SkipOnUpstreamFailure(t, %s) before the check, so somebody\n"+
				"  else's outage skips this test rather than failing the build.",
				filepath.ToSlash(rel), lineOf(body, m[0]), call, name, name)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if checked == 0 {
		t.Fatal("no externally-tagged test files examined; the walk root or the tag pattern is wrong")
	}

	t.Logf("%d externally-tagged test files checked, %d offenders", checked, offenders)
}
