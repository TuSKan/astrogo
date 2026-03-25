package catalog

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"sync"
	_ "embed"
)

//go:generate go run ../internal/tools/download.go https://raw.githubusercontent.com/mattiaverga/OpenNGC/master/database_files/NGC.csv data/NGC.csv
//go:embed data/NGC.csv
var ngcCSV []byte

//go:generate go run ../internal/tools/download.go https://raw.githubusercontent.com/mattiaverga/OpenNGC/master/database_files/addendum.csv data/addendum.csv
//go:embed data/addendum.csv
var addendumCSV []byte

var (
	targetIndex map[string]*DeepSkyTarget
	ngcOnce     sync.Once
	ngcInitErr  error
)

// normalize purges spaces, hyphens, and underscores flattening queries natively.
func normalize(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "_", "")
	return s
}

func loadCatalog() {
	targetIndex = make(map[string]*DeepSkyTarget)

	ngcTargets, err := ParseNGC(bytes.NewReader(ngcCSV))
	if err != nil {
		ngcInitErr = fmt.Errorf("failed to parse structured NGC schema natively: %w", err)
		return
	}

	addendumTargets, err := ParseNGC(bytes.NewReader(addendumCSV))
	if err != nil {
		ngcInitErr = fmt.Errorf("failed to parse structurally bound addendums intrinsically: %w", err)
		return
	}

	populateIndex := func(targets []DeepSkyTarget) {
		for i := range targets {
			ptr := &targets[i]

			// Index 1: Primary ID cleanly mapped
			if ptr.ID != "" {
				targetIndex[normalize(ptr.ID)] = ptr
			}

			// Index 2: Explicit Messier binding resolving "42" into "M42" seamlessly
			if ptr.Messier != "" {
				cleanM := strings.TrimLeft(ptr.Messier, "0")
				if cleanM != "" {
					targetIndex[normalize("M"+cleanM)] = ptr
				}
			}

			// Index 3: Comma-separated Identifiers
			if ptr.Identifiers != "" {
				idents := strings.Split(ptr.Identifiers, ",")
				for _, id := range idents {
					if strings.TrimSpace(id) != "" {
						targetIndex[normalize(id)] = ptr
					}
				}
			}

			// Index 4: Comma-separated Common names
			if ptr.CommonNames != "" {
				names := strings.Split(ptr.CommonNames, ",")
				for _, cn := range names {
					if strings.TrimSpace(cn) != "" {
						targetIndex[normalize(cn)] = ptr
					}
				}
			}
		}
	}

	populateIndex(ngcTargets)
	populateIndex(addendumTargets)

	if len(targetIndex) == 0 {
		ngcInitErr = errors.New("OpenNGC unified lookup database is catastrophically null post generation")
	}
}

// Lookup provides robust multi-alias indexing strictly resolving normalized string formats over O(1) properties.
func Lookup(query string) (*DeepSkyTarget, error) {
	ngcOnce.Do(loadCatalog)

	if ngcInitErr != nil {
		return nil, ngcInitErr
	}

	normQuery := normalize(query)
	if target, exists := targetIndex[normQuery]; exists {
		return target, nil
	}

	return nil, fmt.Errorf("target not found: %q", query)
}
