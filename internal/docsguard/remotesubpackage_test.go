package docsguard_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The import prefixes this guard is about.
const (
	remotePkg     = "github.com/TuSKan/astrogo/remote"
	remoteSubPkg  = remotePkg + "/"
	remoteDirName = "remote"
)

// optInSubpackages are the remote subpackages a program outside remote/ may
// name, and only as a blank import.
//
// Each is a registration and nothing else — it exports no symbol at all, so
// importing one for a name is already impossible and importing it blank is the
// only thing it is for. The storage ones register a URL scheme with gocloud's
// opener registry; remote/eop registers the IERS loader with astrogo/time.
//
// They are opt-in rather than always-on for the same reason in both cases: what
// they pull in should not be in a build that never asked. An s3:// scheme costs
// the AWS SDK, and EOP data costs somebody else's bandwidth on somebody else's
// schedule.
var optInSubpackages = map[string]bool{
	remotePkg + "/file/s3":    true,
	remotePkg + "/file/gcs":   true,
	remotePkg + "/file/azure": true,
	remotePkg + "/file/sftp":  true,
	remotePkg + "/eop":        true,
}

// TestSubpackagesAreNotImportedDirectly keeps remote's front door the only door.
//
// # The rule
//
// remote/file and remote/api move bytes. remote decides whether bytes may move
// at all: which endpoint, whether the process is offline, whether a download
// was consented to, where the result is cached. Every one of those questions is
// answered in remote and nowhere else, so a package that imports a subpackage
// directly is not taking a shortcut — it is reaching past the gate.
//
// The failure that makes this structural rather than advisory is quiet. A
// provider calling file.Open on an endpoint URL gets working code: the bytes
// arrive, the tests pass, and remote.SetOffline stops meaning anything for that
// one call site. Nothing reports it. It surfaces as a library that talks to the
// network in a program that asked it not to.
//
// # The exceptions
//
// The packages in [optInSubpackages] are blank imports and export nothing at
// all — a URL scheme registration, or the IERS EOP loader. A program that wants
// one has to name it, because what each pulls in does not belong in a build
// that never asked. Blank only: importing one for a symbol would mean it had
// grown one.
//
// # Why the whole module, tests included
//
// A test importing remote/file is how the boundary erodes in practice — it is
// scaffolding, nobody argues with it, and it is the first citation the next
// person finds when they need the same thing in production. remote's own tree
// is exempt because it is the implementation.
func TestSubpackagesAreNotImportedDirectly(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)

	var (
		offenders []string
		scanned   int
	)

	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			switch d.Name() {
			case ".git", "testdata", "scratchpad":
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}

		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return fmt.Errorf("relative path for %s: %w", p, relErr)
		}

		// remote's own tree is the implementation and may import itself.
		if first, _, _ := strings.Cut(rel, string(filepath.Separator)); first == remoteDirName {
			return nil
		}

		scanned++

		bad, parseErr := forbiddenRemoteImports(p, rel)
		if parseErr != nil {
			return parseErr
		}

		offenders = append(offenders, bad...)

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	// A walk that silently stopped early would pass this test by finding
	// nothing, which is the one way a structural guard fails without saying so.
	if scanned < 400 {
		t.Fatalf("only %d Go files scanned outside remote/; the walk is not reaching the module", scanned)
	}

	for _, o := range offenders {
		t.Errorf("%s\n"+
			"  remote/file and remote/api are internal to remote. Everything they do is "+
			"reachable from remote itself — OpenBucket, Save, NewReaderAt, GetFile, "+
			"NewAPIClient — and going around it skips the endpoint registry, offline "+
			"mode and the download consent gate, silently and only for that call site.\n"+
			"  The only exceptions are blank imports of the opt-in registrations: "+
			"remote/file/{s3,gcs,azure,sftp} and remote/eop.", o)
	}
}

// forbiddenRemoteImports returns one message per offending import in the file
// at p, described by its repo-relative path rel.
func forbiddenRemoteImports(p, rel string) ([]string, error) {
	fset := token.NewFileSet()

	f, err := parser.ParseFile(fset, p, nil, parser.ImportsOnly)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}

	var bad []string

	for _, spec := range f.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return nil, fmt.Errorf("import path in %s: %w", p, err)
		}

		if !strings.HasPrefix(path, remoteSubPkg) {
			continue
		}

		where := rel + ":" + strconv.Itoa(fset.Position(spec.Pos()).Line)

		if optInSubpackages[path] {
			if !isBlankImport(spec) {
				bad = append(bad, where+": imports "+path+" for a symbol")
			}

			continue
		}

		bad = append(bad, where+": imports "+path)
	}

	return bad, nil
}

// isBlankImport reports whether spec is spelled `_ "path"`.
func isBlankImport(spec *ast.ImportSpec) bool {
	return spec.Name != nil && spec.Name.Name == "_"
}
