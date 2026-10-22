package docsguard_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// geodeticFromDegrees matches a NewGeodetic call whose longitude is written in
// degrees, angle.Deg(x) or angle.Zero(): the call NewEarthLocation exists
// for.
var geodeticFromDegrees = regexp.MustCompile(`NewGeodetic\(\s*angle\.(?:Deg\(|Zero\(\))`)

// TestExamplesBuildSitesWithNewEarthLocation keeps the code a user copies on
// coord.NewEarthLocation.
//
// # Why
//
// NewEarthLocation(lat, lon, heightMeters) takes the numbers as a GPS receiver
// or a map gives them, and needs neither angle nor unit imported.
// NewGeodetic(angle.Deg(lon), angle.Deg(lat), height) builds the same value
// with two imports and longitude first, and that order is easy to get
// backward. Two of the three Jesus examples and docs/JESUS.md had: they put
// Jerusalem at longitude 31.77°, latitude 35.21°, some 470 km off, while the
// third had it right. In an example the reader copies, the order a map gives
// is the one to show.
//
// # Scope
//
// What a user reads to learn the API: the examples/ module, every Markdown
// file, and every Go file that holds an Example function. Library code and
// ordinary tests may build a Geodetic either way.
func TestExamplesBuildSitesWithNewEarthLocation(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)

	var checked int

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable path is skipped, not fatal
		}

		if d.IsDir() {
			if name := d.Name(); name == ".git" || name == "testdata" || name == "node_modules" {
				return filepath.SkipDir
			}

			return nil
		}

		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil //nolint:nilerr // an unrelatable path is skipped, not fatal
		}

		rel = filepath.ToSlash(rel)
		if rel == "CHANGELOG.md" || (!strings.HasSuffix(rel, ".go") && !strings.HasSuffix(rel, ".md")) {
			return nil
		}

		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil //nolint:nilerr // an unreadable file is skipped, not fatal
		}

		text := string(data)

		userFacing := strings.HasPrefix(rel, "examples/") || strings.HasSuffix(rel, ".md") ||
			(strings.HasSuffix(rel, "_test.go") && strings.Contains(text, "\nfunc Example"))
		if !userFacing {
			return nil
		}

		checked++

		for i, line := range strings.Split(text, "\n") {
			if geodeticFromDegrees.MatchString(line) {
				t.Errorf("%s:%d: a site in degrees in code a user copies; write coord.NewEarthLocation(lat, lon, heightMeters).\n"+
					"  Mind the order: latitude first, the reverse of NewGeodetic's.", rel, i+1)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if checked < 50 {
		t.Fatalf("checked only %d user-facing files; the walk is not seeing the repository", checked)
	}
}
