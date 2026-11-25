package examples_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// readmeProgram matches the README's first program and the printed output
// that follows it: a Go block that is a whole program, its package comment
// included, then a bare block.
var readmeProgram = regexp.MustCompile("(?s)```go\n((?://[^\n]*\n)*package main\n.*?)```\n\n```\n(.*?)```")

// readmeBlocks returns the README's first program and the output it shows.
func readmeBlocks(t *testing.T) (program, output string) {
	t.Helper()

	readme, err := os.ReadFile(filepath.Join("..", "README.md"))
	if err != nil {
		t.Fatalf("read README: %v", err)
	}

	m := readmeProgram.FindStringSubmatch(lf(string(readme)))
	if m == nil {
		t.Fatal("the README has no program followed by its output; readmeProgram no longer matches it")
	}

	return m[1], m[2]
}

// lf returns s with Windows line endings made Unix ones, so a checkout that
// converts them compares as the repository holds it.
func lf(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }

// TestREADMEProgramIsExample00 holds the README's first program to
// 00_readme/main.go, byte for byte, so the program the output test runs is
// the one the README shows (#1188).
func TestREADMEProgramIsExample00(t *testing.T) {
	t.Parallel()

	program, _ := readmeBlocks(t)

	src, err := os.ReadFile(filepath.Join("00_readme", "main.go"))
	if err != nil {
		t.Fatal(err)
	}

	if got := lf(string(src)); got != program {
		t.Errorf("00_readme/main.go is not the README's first program; copy one onto the other.\nREADME:\n%s\n00_readme/main.go:\n%s", program, got)
	}
}

// TestREADMEOutputIsWhatTheProgramPrints runs 00_readme against a fresh cache
// and requires what it prints to be the README's output block exactly. The
// README showed Mars rising at 07:46 and setting at 19:49 for two releases
// after #568 moved both, and nothing noticed (#1188). Standard error is not
// compared: with no Earth orientation loaded the program warns there, which
// the README does not show.
func TestREADMEOutputIsWhatTheProgramPrints(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode: running the README's program takes a moment")
	}

	_, want := readmeBlocks(t)

	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("go not on PATH: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, goBin, "run", "./00_readme")

	cmd.Env = append(os.Environ(), "ASTROGO_CACHE_DIR="+cacheURL(t.TempDir()), "NO_COLOR=1")

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("00_readme failed: %v\n%s", err, out)
	}

	if got := lf(string(out)); got != want {
		t.Errorf("the README shows\n%s\nbut 00_readme prints\n%s", want, got)
	}
}

// cacheURL is dir as the file URL ASTROGO_CACHE_DIR holds: "file:///tmp/x" on
// Unix, "file:///C:/Users/x" on Windows.
func cacheURL(dir string) string {
	dir = filepath.ToSlash(dir)
	if !strings.HasPrefix(dir, "/") {
		dir = "/" + dir
	}

	return "file://" + dir
}
