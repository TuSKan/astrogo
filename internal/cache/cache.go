// Package cache provides a unified caching layer for astrogo's
// runtime-downloaded data files (IERS EOP, JPL SPK/LSK kernels, etc.).
//
// All cached files are stored under os.UserCacheDir()/astrogo/…
// (e.g. ~/.cache/astrogo on Linux, %LocalAppData%/astrogo on Windows,
// ~/Library/Caches/astrogo on macOS).
//
// If UserCacheDir is unavailable, os.TempDir() is used as a fallback.
package cache

import (
	"fmt"
	"os"
	"path/filepath"
)

const appName = "astrogo"

// Dir returns the absolute path to the astrogo cache directory for
// the given subsystem (e.g. "iers", "jpl", "catalog").
// The directory is created if it does not yet exist.
func Dir(subsystem string) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}

	dir := filepath.Join(base, appName, subsystem)

	err = os.MkdirAll(dir, 0o755)
	if err != nil {
		return dir, fmt.Errorf("cache: mkdir %s: %w", dir, err)
	}

	return dir, nil
}

// Path returns the absolute path for a cached file identified by
// subsystem and relative name (e.g. "jpl", "planets/de442.bsp").
// Parent directories are created automatically.
func Path(subsystem, name string) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}

	p := filepath.Join(base, appName, subsystem, filepath.FromSlash(name))

	err = os.MkdirAll(filepath.Dir(p), 0o755)
	if err != nil {
		return "", fmt.Errorf("cache: mkdir: %w", err)
	}

	return p, nil
}
