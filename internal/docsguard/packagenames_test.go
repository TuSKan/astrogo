package docsguard_test

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestNoTwoPackagesShareAName holds #115's renames: three packages were named
// plan, three jpl and two fits, so an importer of two of them had to alias
// one, and pkg.go.dev listed several packages under one name. Every library
// package now has a name of its own. Programs are exempt, since every command
// is package main.
func TestNoTwoPackagesShareAName(t *testing.T) {
	root := filepath.Join("..", "..")

	for _, shared := range sharedPackageNames(moduleSymbols(t).dirsByName) {
		t.Errorf("%s; give each its own name, since an importer of two would have to alias one", shared)
	}

	// The check must be able to fail: a second directory declaring plan,
	// planted, is reported.
	planted := map[string][]string{
		"plan": {filepath.Join(root, "plan"), filepath.Join(root, "planted", "plan")},
		"main": {filepath.Join(root, "examples", "a"), filepath.Join(root, "examples", "b")},
	}
	if got := sharedPackageNames(planted); len(got) != 1 || !strings.HasPrefix(got[0], "package plan ") {
		t.Errorf("a planted second package plan gave %q, want it reported, and main left alone", got)
	}
}

// sharedPackageNames describes every name, other than main, that more than
// one directory declares.
func sharedPackageNames(dirsByName map[string][]string) []string {
	var out []string

	for name, dirs := range dirsByName {
		if name == "main" || len(dirs) < 2 {
			continue
		}

		sorted := append([]string(nil), dirs...)
		sort.Strings(sorted)
		out = append(out, "package "+name+" is declared by "+strings.Join(sorted, ", "))
	}

	sort.Strings(out)

	return out
}
