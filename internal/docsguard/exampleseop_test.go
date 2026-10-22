package docsguard_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// consentStatement matches a statement granting download consent, as
// opposed to the call quoted in a comment or a printed hint.
var consentStatement = regexp.MustCompile(`(?m)^\s*remote\.EnableDownloads\((.*)\)\s*(//.*)?$`)

// TestOnlineExamplesLoadEarthOrientation: every example that goes online,
// one with a statement calling remote.EnableDownloads, blank-imports
// remote/eop and consents to the IERS bulletin, so its sites sit where the
// Earth's rotation actually put them.
//
// # Why
//
// Without the import every epoch uses zero DUT1 and polar motion, up to
// 13.5″ on a topocentric place, and the program says only so in a warning. The
// import alone is not enough: the loader fetches the bulletin only with
// consent, and most examples consent to just the endpoints they name. So both
// are checked. An example that grants consent with no endpoints at all covers
// the bulletin already.
func TestOnlineExamplesLoadEarthOrientation(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)

	var online int

	err := filepath.WalkDir(filepath.Join(root, "examples"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable path is skipped, not fatal
		}

		if d.IsDir() || d.Name() != "main.go" {
			return nil
		}

		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil //nolint:nilerr // an unreadable file is skipped, not fatal
		}

		text := string(data)

		consents := consentStatement.FindAllStringSubmatch(text, -1)
		if len(consents) == 0 {
			return nil
		}

		online++

		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)

		if !strings.Contains(text, `_ "github.com/TuSKan/astrogo/remote/eop"`) {
			t.Errorf("%s goes online but does not import remote/eop: every epoch would use zero Earth orientation.\n"+
				"  Add _ \"github.com/TuSKan/astrogo/remote/eop\" to its imports.", rel)
		}

		covered := false

		for _, c := range consents {
			args := strings.Split(c[1], ",")
			if len(args) == 1 || strings.Contains(c[1], "IERSFinals2000A") {
				covered = true
			}
		}

		if !covered {
			t.Errorf("%s imports remote/eop but never consents to the bulletin it fetches.\n"+
				"  Add remote.EnableDownloads(0, remote.IERSFinals2000A) beside its other consent.", rel)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if online < 10 {
		t.Fatalf("found only %d online examples; the walk is not seeing examples/", online)
	}
}
