package iers

import (
	"fmt"
	"io/fs"
)

// LoadFS parses a finals2000A-format file reached through an io/fs.FS —
// a local directory via os.DirFS, an embed.FS bundled by a downstream
// application, or any other stdlib-compatible filesystem — and registers
// it as the global EOP model. This is the one offline/explicit-control
// path: pre-seed a file (e.g. one downloaded once via FetchNow and copied
// into a deployment image) and load it without any network access.
func LoadFS(fsys fs.FS, name string) error {
	f, err := fsys.Open(name)
	if err != nil {
		return fmt.Errorf("iers: open %s: %w", name, err)
	}

	defer f.Close() //nolint:errcheck // read-only file, close error is not actionable here

	table, err := ParseFinals2000A(f)
	if err != nil {
		return fmt.Errorf("iers: parse %s: %w", name, err)
	}

	RegisterModel(table)

	return nil
}
