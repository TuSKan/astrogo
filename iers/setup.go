package iers

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// ErrEmbeddedUnavailable indicates UseEmbedded was called on a build where
// `go generate ./iers/...` never ran, so there is no embedded finals2000A
// snapshot to fall back to.
var ErrEmbeddedUnavailable = errors.New("iers: no embedded EOP data available")

// LoadFile parses a local finals2000A-format file and registers it as the
// global EOP model — the offline/air-gapped path: pre-seed a file (e.g.
// downloaded once via `go generate ./iers/...` or FetchNow and copied into
// a deployment image) and load it without any network access.
func LoadFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("iers: open %s: %w", path, err)
	}

	defer f.Close() //nolint:errcheck // read-only file, close error is not actionable here

	table, err := ParseFinals2000A(f)
	if err != nil {
		return fmt.Errorf("iers: parse %s: %w", path, err)
	}

	RegisterModel(table)

	return nil
}

// LoadFS is LoadFile for a finals2000A-format file reached through an
// io/fs.FS — e.g. an embed.FS bundled by a downstream application, or any
// other stdlib-compatible filesystem.
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

// UseEmbedded re-registers astrogo's own build-time embedded finals2000A
// snapshot as the global model, undoing a prior LoadFile/LoadFS/FetchNow.
// Returns an error if `go generate ./iers/...` was never run for this
// build (no embedded data available).
func UseEmbedded() error {
	if len(FinalsData) == 0 {
		// loadEmbedded may not have run yet (lazy) — force it once so a
		// build that generated data still finds it here.
		loadOnce.Do(loadEmbedded)
	}

	if len(FinalsData) == 0 {
		return fmt.Errorf("iers: no embedded EOP data (run `go generate ./iers/...`): %w", ErrEmbeddedUnavailable)
	}

	table, err := ParseFinals2000A(bytes.NewReader(FinalsData))
	if err != nil {
		return fmt.Errorf("iers: parse embedded EOP data: %w", err)
	}

	RegisterModel(table)

	return nil
}
