package docsguard_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// nolintDirective matches a suppression and captures the linter list it names.
var nolintDirective = regexp.MustCompile(`//nolint:([a-zA-Z0-9_,]+)`)

// disabledLinter matches one entry of .golangci.yml's linters.disable block.
var disabledLinter = regexp.MustCompile(`^\s*-\s+(\S+)`)

// TestNoNolintForADisabledLinter checks that every //nolint directive names a
// linter that actually runs.
//
// # Why this is worse than clutter
//
// .golangci.yml runs `default: all` with a documented disable list, so a
// linter named in that list never fires — and a //nolint directive naming it
// suppresses nothing. There were 42 such directives for gochecknoglobals
// alone, 29% of every suppression in the tree.
//
// CLAUDE.md permits //nolint "only when locally scoped with a documented
// reason", so each of those carried a written justification for silencing
// nothing. That is the damaging part. A reader cannot tell the live
// suppressions from the dead ones, and the rule that was supposed to make
// each one deliberate instead made a wall of them look considered. Five were
// added by an assistant following that rule without checking the config: the
// process made the output look more rigorous, not less.
//
// # Why the check is against the disable list
//
// A dead directive is invisible to golangci-lint itself. nolintlint reports an
// *unused* directive — one whose linter ran and had nothing to say — but a
// directive for a disabled linter is simply skipped, so nothing complains. The
// config is the only place the answer lives.
//
// This does not validate that a name is a real linter; a typo would still pass.
// It catches the failure that actually happened, which is a directive that was
// correct when written and became dead when the linter was disabled.
func TestNoNolintForADisabledLinter(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")

	disabled := disabledLinters(t, filepath.Join(root, ".golangci.yml"))
	if len(disabled) < 10 {
		t.Fatalf("parsed only %d disabled linters from .golangci.yml; the disable "+
			"block is no longer being found, so this test would pass vacuously",
			len(disabled))
	}

	var checked, directives int

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

		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil //nolint:nilerr // an unrelatable path is skipped, not fatal
		}

		rel = filepath.ToSlash(rel)

		// This file writes the directives it is describing.
		if strings.HasSuffix(rel, "internal/docsguard/nolint_test.go") {
			return nil
		}

		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil //nolint:nilerr // an unreadable file is skipped, not fatal
		}

		checked++

		n := 0

		for line := range strings.SplitSeq(string(data), "\n") {
			n++

			for _, m := range nolintDirective.FindAllStringSubmatch(line, -1) {
				for linter := range strings.SplitSeq(m[1], ",") {
					directives++

					if reason, dead := disabled[linter]; dead {
						t.Errorf("%s:%d suppresses %q, which .golangci.yml disables"+
							"%s.\n  The directive silences nothing, and its documented "+
							"reason reads as a considered decision. Delete it.",
							rel, n, linter, reason)
					}
				}
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if checked < 200 {
		t.Fatalf("only %d Go files scanned; the walk is not reaching the module", checked)
	}

	t.Logf("%d files scanned, %d nolint directives, %d disabled linters in config",
		checked, directives, len(disabled))
}

// disabledLinters reads the linters.disable block of .golangci.yml, mapping
// each name to the trailing comment that justifies disabling it.
func disabledLinters(t *testing.T, path string) map[string]string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	out := map[string]string{}
	inBlock := false

	for line := range strings.SplitSeq(string(data), "\n") {
		trimmed := strings.TrimSpace(line)

		if trimmed == "disable:" {
			inBlock = true
			continue
		}

		if !inBlock {
			continue
		}

		// A blank line or comment stays inside the block; anything else at or
		// left of the block's own indentation ends it.
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		m := disabledLinter.FindStringSubmatch(line)
		if m == nil {
			break
		}

		name := m[1]

		reason := ""
		if _, after, found := strings.Cut(line, "#"); found {
			reason = " — " + strings.TrimSpace(after)
		}

		out[name] = reason
	}

	return out
}
