package docsguard_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// bucketWrite matches a write through a gocloud bucket handle.
//
// Both spellings are covered because both stage the same way: WriteAll is
// NewWriter plus a copy and a close, and it is the shorter one, which is why
// it is the one that gets reached for.
var bucketWrite = regexp.MustCompile(`\.(WriteAll|NewWriter)\(`)

// bucketWriteAllowed are the files that may write to a bucket directly.
//
// remote/file/file.go is the funnel — Save takes the staging lock. The lock and
// resume path beside it writes with WriterOptions that Save has no parameter
// for (IfNotExist for the lock object, source-ETag metadata for the staged
// download), and needs no lock of its own: every one of those writes runs
// holding AcquireLock, which #245 made exclusive within the process as well as
// across them.
var bucketWriteAllowed = map[string]bool{
	filepath.Join("remote", "file", "file.go"):        true,
	filepath.Join("remote", "file", "lock_resume.go"): true,
}

// TestBucketWritesGoThroughTheStagingLock keeps #241 from coming back by the
// front door.
//
// # What the rule protects
//
// fileblob stages every write in os.TempDir under a name built from the key's
// basename and a nanosecond clock, and on Windows that clock does not move —
// 2000 consecutive reads returned one distinct value. Two writers of one file
// name therefore agree on the staging path, and one renames it out from under
// the other. The write lock [file.Save] holds serialises them.
//
// A new caller reaching for bucket.WriteAll instead gets code that works
// everywhere its author runs it and fails one time in seven on Windows,
// against a key nothing else touched. That is not a failure anybody traces
// back to the line that caused it, which is the argument for a structural
// guard over a comment.
//
// # Why only production files
//
// Test scaffolding writes buckets directly in a dozen places and mostly writes
// distinct names, where there is nothing to collide. The three that did — the
// spk discard tests, all t.Parallel, all seeding "de440s.bsp" — are what filed
// #241, and they now go through Save. Widening this to tests would be a large
// mechanical change for a small gain, and the gain is stated here instead:
// concurrent test writers of one file name must use file.Save.
func TestBucketWritesGoThroughTheStagingLock(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)

	var offenders []string

	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			switch d.Name() {
			case ".git", "testdata", "examples", "scratchpad":
				return filepath.SkipDir
			}

			return nil
		}

		name := d.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}

		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return fmt.Errorf("relative path for %s: %w", p, relErr)
		}

		if bucketWriteAllowed[rel] {
			return nil
		}

		src, readErr := os.ReadFile(p)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", p, readErr)
		}

		for i, line := range strings.Split(string(src), "\n") {
			if bucketWrite.MatchString(line) {
				offenders = append(offenders,
					rel+":"+strconv.Itoa(i+1)+": "+strings.TrimSpace(line))
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	for _, o := range offenders {
		t.Errorf("%s writes to a bucket directly.\n"+
			"  Use file.Save, which holds the staging lock. fileblob names its temp file "+
			"from the key's basename and a clock that does not move on Windows, so two "+
			"writers of one name rename it out from under each other (#241). If this file "+
			"genuinely needs WriterOptions, add it to bucketWriteAllowed and say there "+
			"what serialises it.", o)
	}
}
