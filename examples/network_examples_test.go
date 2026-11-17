package examples_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/remote"
)

// networkExamples names, for each example the README's Network column says
// reaches the network, the endpoints it reaches. TestNetworkExamplesRun, under
// the network tag, runs every one nightly and skips one whose endpoints
// cannot be reached, as the network tests skip; TestNetworkExamplesAreListed
// holds this table to the README's in both directions (#1188).
var networkExamples = map[string][]remote.EndpointID{
	"05_resolve_name":          {remote.SIMBAD, remote.MAST, remote.OpenNGC, remote.IERSFinals2000A},
	"09_geometry_events":       {remote.NAIFSPK, remote.NAIFLSK, remote.IERSFinals2000A},
	"11_skyfield_verify":       {remote.NAIFSPK, remote.NAIFLSK, remote.IERSFinals2000A},
	"12_satellite_tracking":    {remote.CelesTrak, remote.IERSFinals2000A},
	"15_target_details":        {remote.SIMBAD, remote.CelesTrak, remote.OpenNGC, remote.NAIFSPK, remote.NAIFLSK, remote.IERSFinals2000A},
	"18_sky_brightness":        {remote.GaiaStarMap, remote.SFDDustMap, remote.IERSFinals2000A},
	"20_whats_visible_tonight": {remote.SIMBAD, remote.OpenNGC, remote.JPLSBDBQuery, remote.JPLHorizonsSPK, remote.NAIFSPK, remote.NAIFLSK, remote.IERSFinals2000A},
	"22_kepler_propagator":     {remote.JPLSBDB},
	"25_sky_brightness_compare": {
		remote.GaiaStarMap, remote.SFDDustMap, remote.CALSPEC, remote.IERSFinals2000A,
	},
}

// exampleRow matches a row of the README's examples table, capturing the
// example's name and its Network column:
//
//	| [`05_resolve_name`](examples/05_resolve_name/) | ... | SIMBAD and MAST, live; OpenNGC, once |
var exampleRow = regexp.MustCompile("(?m)^\\| \\[`([0-9]{2}_[a-z0-9_]+)`\\]\\(examples/[^)]+\\) \\|.*\\| ([^|]+?) \\|\\s*$")

// readmeNetworkExamples returns the examples the README says reach the
// network: every row whose Network column is not "none".
func readmeNetworkExamples(t *testing.T) []string {
	t.Helper()

	readme, err := os.ReadFile(filepath.Join("..", "README.md"))
	if err != nil {
		t.Fatalf("read README: %v", err)
	}

	var names []string

	for _, m := range exampleRow.FindAllStringSubmatch(string(readme), -1) {
		if strings.TrimSpace(m[2]) != "none" {
			names = append(names, m[1])
		}
	}

	// As for offlineExamples: almost nothing means the pattern stopped
	// matching the table, not that the examples went offline.
	if len(names) < 5 {
		t.Fatalf("found %d network examples in the README; the table no longer matches exampleRow", len(names))
	}

	return names
}

// TestNetworkExamplesAreListed holds networkExamples to the README: every
// example the README says reaches the network has its endpoints listed, so
// the nightly run covers it, and every listed example is one the README says
// reaches the network, with a directory that exists.
func TestNetworkExamplesAreListed(t *testing.T) {
	t.Parallel()

	readme := readmeNetworkExamples(t)

	for _, name := range readme {
		if _, ok := networkExamples[name]; !ok {
			t.Errorf("the README says %s reaches the network, but networkExamples does not list it", name)
		}
	}

	for name, ids := range networkExamples {
		if !slices.Contains(readme, name) {
			t.Errorf("networkExamples lists %s, which the README does not say reaches the network", name)
		}

		// The directory holds at least one program, or this fails.
		examplePrograms(t, name)

		for _, id := range ids {
			if _, ok := remote.Lookup(id); !ok {
				t.Errorf("%s lists endpoint %q, which remote does not register", name, id)
			}
		}
	}
}

// examplePrograms returns the main packages of the example in dir, as go run
// takes them: the directory itself, or each subdirectory holding a program,
// as 15_target_details holds one per kind of target.
func examplePrograms(t *testing.T, dir string) []string {
	t.Helper()

	var pkgs []string

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && d.Name() == "main.go" {
			pkgs = append(pkgs, "./"+filepath.ToSlash(filepath.Dir(path)))
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}

	if len(pkgs) == 0 {
		t.Fatalf("%s holds no program", dir)
	}

	return pkgs
}
