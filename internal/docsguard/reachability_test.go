package docsguard_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// dialCall matches a raw connection attempt: net.Dial, net.DialTimeout, or a
// net.Dialer's own DialContext.
var dialCall = regexp.MustCompile(`\bnet\.Dial(Timeout|Context)?\(|\bnet\.Dialer\b`)

// TestOnlyTestutilProbesReachability keeps every suite's "is the service
// there?" question going through one implementation.
//
// # Why this is worth a guard
//
// A reachability probe is the one piece of test plumbing whose bugs are
// invisible. Everything else fails loudly; this fails as SKIP, which reads as
// a pass in every summary that exists, and the suite it disables is by
// definition the one nobody is watching.
//
// It has happened twice, in two different hand-rolled probes:
//
//   - Go's dual-stack dialer spent the whole budget on an AAAA lookup for a
//     record that did not exist, and every IRSA-dependent suite — the SFD dust
//     map and the GAMBONS all-sky comparison among them — skipped on a host
//     answering in 250 ms over IPv4.
//   - plan/usno_test.go cached a failed probe in a sync.Once, so one dropped
//     SYN retired all fourteen TestUSNO_* functions for the rest of the binary.
//     Measured while the host answered curl in 2.2 s: a 5 s dial overran to
//     17.3 s and failed, and the next one connected in 140 ms (#225).
//
// [github.com/TuSKan/astrogo/internal/testutil.Reachable] carries the fix for
// both. A second implementation does not, and cannot be assumed to acquire the
// third fix either.
//
// # Scope
//
// Test files only, and testutil itself is the sanctioned implementation.
// Library code dialling is a different question this does not speak to —
// nothing in astrogo does it today, and `remote` is where it would belong.
func TestOnlyTestutilProbesReachability(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)

	var offenders []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable path is skipped, not fatal
		}

		if info.IsDir() {
			switch info.Name() {
			case ".git", "node_modules", "testutil":
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}

		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil //nolint:nilerr // unreadable is not an offence
		}

		for i, line := range strings.Split(string(src), "\n") {
			// A comment explaining a past probe bug is not a probe. The
			// guard is about code that dials, and usno_test.go's own doc
			// comment quotes the call it replaced.
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}

			if dialCall.MatchString(line) {
				rel, relErr := filepath.Rel(root, path)
				if relErr != nil {
					rel = path
				}

				offenders = append(offenders, filepath.ToSlash(rel)+":"+itoa(i+1))
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if len(offenders) > 0 {
		t.Errorf("test files dial directly instead of using testutil.Reachable:\n  %s\n\n"+
			"  A hand-rolled probe is how two suites came to skip against hosts that were "+
			"answering, and a skip reads as a pass. testutil.Reachable already retries over "+
			"IPv4 when the dual-stack dialer stalls; a second implementation starts without "+
			"that and without whatever is learned next.",
			strings.Join(offenders, "\n  "))
	}
}
