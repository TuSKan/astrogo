package spk

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	gofs "github.com/ungerik/go-fs"

	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

// DOC: https://ssd-api.jpl.nasa.gov/doc/horizons.html
// SELECT: https://ssd.jpl.nasa.gov/horizons/manual.html#select

// HorizonsResult is a single result from the Horizons API.
type HorizonsResult struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Designation string   `json:"designation"`
	Aliases     []string `json:"aliases"`
}

// HorizonsResponse is the response from the Horizons API.
type HorizonsResponse struct {
	Result    string `json:"result"`
	Signature struct {
		Source  string `json:"source"`
		Version string `json:"version"`
	} `json:"signature"`
	Spk       string `json:"spk"`
	SpkFileID string `json:"spk_file_id"`
}

// maxNumberedAsteroidRecord is Horizons' own documented ceiling for a bare
// numeric COMMAND to resolve directly as a numbered-asteroid record ("IAU
// numbered asteroids: 1-895910", from its own out-of-bounds error message
// — confirmed live, not copied from documentation). Every real SBDB
// SPK-ID uses a different numbering scheme entirely (asteroids:
// 2000000+number; comets: their own 1000000+/900000+ ranges) and always
// exceeds this ceiling, so a bare attempt for one is guaranteed to fail.
const maxNumberedAsteroidRecord = 895910

// commandCandidates returns the ordered Horizons COMMAND strings CacheAPI
// should try for kernel, stopping at the first one that produces a direct
// SPK. The bare form is skipped when kernel is unambiguously outside the
// numbered-asteroid record range (a real SBDB SPK-ID always is) — trying
// it first would just be a wasted, guaranteed-to-fail round trip before
// falling through to DES=; a genuinely short designation like "433" still
// gets the bare form first, since "DES=433;" alone resolves to a
// different, unrelated body (confirmed live — see CacheAPI's doc comment).
func commandCandidates(kernel string) []string {
	des := "DES=" + kernel + ";"
	desCAP := des + "CAP"

	if n, err := strconv.Atoi(kernel); err == nil && n > maxNumberedAsteroidRecord {
		return []string{des, desCAP}
	}

	return []string{kernel, des, desCAP}
}

// CacheAPI caches an SPK file from JPL Horizons if it doesn't exist.
//
// It automatically handles:
// - Directory creation
// - File existence check
// - Base64 decoding
// - File writing
// - Reader creation
// - Error handling
func CacheAPI(ctx context.Context, kernel string, startTime, endTime time.Time, path string) ([]*Reader, error) {
	var readers []*Reader

	spkFile := kernel + ".bsp"
	spkPath := filepath.Join(path, spkFile)

	if gofs.File(spkPath).Exists() {
		// Already exists, just return reader
		ra, err := openReaderAt(gofs.File(spkPath))
		if err != nil {
			return nil, fmt.Errorf("jpl: failed to open cached SPK %s: %w", spkPath, err)
		}

		reader, err := NewReader(ra)
		if err != nil {
			return nil, fmt.Errorf("jpl: failed to create reader for %s: %w", spkPath, err)
		}

		return []*Reader{reader}, nil
	}

	// Generating and storing a small-body SPK kernel via Horizons is a file
	// download in effect (KB–MB delivered base64-encoded inside the JSON
	// response), so it requires the same explicit consent as any other
	// kernel download: remote.EnableDownloads(remote.JPLHorizons, maxSize).
	if err := remote.CheckDownload(remote.JPLHorizons, spkFile, -1); err != nil {
		return nil, fmt.Errorf("jpl: SPK kernel %s: %w", kernel, err)
	}

	// Horizons' COMMAND field resolves differently depending on exactly
	// how the designation is written — confirmed live against the real
	// API, not inferred from documentation:
	//   - A bare small number within the numbered-asteroid record range
	//     (roughly 1-895910, e.g. "433") resolves directly as that
	//     numbered body — this is what this package's own doc examples
	//     and jpl_test.go's TestSmallBodyEros already rely on, and it
	//     must keep working unchanged.
	//   - A bare number OUTSIDE that range — which every real SBDB
	//     asteroid/comet SPK-ID always is (asteroids: 2000000+number;
	//     comets: their own 1000000+/900000+ ranges) — is rejected
	//     outright ("requested IOBJ=... is out of bounds"). It must be
	//     wrapped as "DES=<id>;" instead.
	//   - Confusingly, "DES=<n>;" for a SHORT number like "433" does NOT
	//     mean the same thing as the bare-number form above — it matched
	//     a completely different, unrelated body in live testing. So the
	//     bare form must be tried first, not skipped in favor of DES=.
	//   - A comet's designation additionally needs Horizons' "closest
	//     apparition" suffix (DES=<id>;CAP) even when DES=<id>; alone
	//     would otherwise be correct for its SPK-ID — without CAP,
	//     Horizons returns an epoch-disambiguation table instead of a
	//     direct SPK.
	//
	// So: try the bare form first (preserves existing behavior and stays
	// a single request for every case that already worked), escalate to
	// DES=<id>; only if that didn't produce a direct SPK, then to
	// DES=<id>;CAP only if that didn't either — except when kernel is
	// unambiguously outside the numbered-asteroid range (see
	// commandCandidates), where the bare attempt is skipped entirely
	// since it's guaranteed to fail: confirmed live that every real SBDB
	// SPK-ID this package is actually called with (from
	// plan.VisibleTonight's Stage 2) takes this path, so skipping the
	// doomed bare attempt roughly halves live network round trips for
	// that real workload.
	var resp *HorizonsResponse

	for _, command := range commandCandidates(kernel) {
		r, err := apiHorizonsRequest(ctx, command, startTime, endTime)
		if err != nil {
			return nil, fmt.Errorf("jpl: failed to get SPK %s: %w", spkPath, err)
		}

		resp = r
		if resp.SpkFileID != "" && resp.Spk != "" {
			break
		}
	}

	if resp.SpkFileID != "" && resp.Spk != "" {
		spkFile = resp.SpkFileID + ".bsp"
		spkPath = filepath.Join(path, spkFile)

		// Decode base64 SPK
		spkData, err := base64.StdEncoding.DecodeString(strings.Map(func(r rune) rune {
			if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
				return -1
			}

			return r
		}, resp.Spk))
		if err != nil {
			return nil, fmt.Errorf("jpl: failed to decode SPK data: %w", err)
		}

		if err := remote.Save(bytes.NewReader(spkData), gofs.File(spkPath)); err != nil {
			return nil, fmt.Errorf("jpl: failed to save SPK %s: %w", spkPath, err)
		}

		ra, err := openReaderAt(gofs.File(spkPath))
		if err != nil {
			return nil, fmt.Errorf("jpl: failed to open SPK %s: %w", spkPath, err)
		}

		reader, err := NewReader(ra)
		if err != nil {
			return nil, fmt.Errorf("jpl: failed to create reader for %s: %w", spkPath, err)
		}

		readers = append(readers, reader)
	} else {
		hRes, err := parseHorizonsResult(resp.Result)
		if err != nil {
			return nil, fmt.Errorf("jpl: failed to parse Horizons result: %w", err)
		}

		for _, r := range hRes {
			sub, err := CacheAPI(ctx, r.ID, startTime, endTime, path)
			if err != nil {
				return nil, fmt.Errorf("jpl: failed to get SPK %s: %w", spkPath, err)
			}

			readers = append(readers, sub...)
		}
	}

	return readers, nil
}

func apiHorizonsRequest(ctx context.Context, command string, startTime, endTime time.Time) (*HorizonsResponse, error) {
	params := url.Values{}
	params.Set("format", "json")
	params.Set("COMMAND", "'"+command+"'")
	params.Set("MAKE_EPHEM", "YES")
	params.Set("EPHEM_TYPE", "SPK")
	params.Set("OBJ_DATA", "NO")
	params.Set("START_TIME", "'"+startTime.Format("2006-01-02 15:04:05.000")+"'")
	params.Set("STOP_TIME", "'"+endTime.Format("2006-01-02 15:04:05.000")+"'")

	client, err := remote.NewClientFor(remote.JPLHorizons)
	if err != nil {
		return nil, fmt.Errorf("jpl: horizons client: %w", err)
	}

	var resp HorizonsResponse
	if err := client.GetJSON(ctx, remote.JPLHorizons, "", params, &resp); err != nil {
		return nil, mapHorizonsStatus(err)
	}

	return &resp, nil
}

// mapHorizonsStatus translates remote's typed HTTP errors into this
// package's documented Horizons sentinels. Non-HTTP errors (registry gate,
// network failures) pass through wrapped.
func mapHorizonsStatus(err error) error {
	var httpErr *remote.HTTPError
	if !errors.As(err, &httpErr) {
		return fmt.Errorf("jpl: horizons request: %w", err)
	}

	switch httpErr.StatusCode {
	case http.StatusBadRequest:
		return ErrHorizonsBadRequest
	case http.StatusMethodNotAllowed:
		return ErrHorizonsMethodNA
	case http.StatusInternalServerError:
		return ErrHorizonsServerError
	case http.StatusServiceUnavailable:
		return ErrHorizonsUnavailable
	default:
		return fmt.Errorf("%w: %d", ErrHorizonsUnexpected, httpErr.StatusCode)
	}
}

// ParseHorizonsSummary parses the tabular search results returned by Horizons when multiple bodies match a query.
func parseHorizonsResult(data string) ([]HorizonsResult, error) {
	result := []HorizonsResult{}
	scanner := bufio.NewScanner(strings.NewReader(data))

	inTable := false

	for scanner.Scan() {
		line := scanner.Text()

		// Look for the separator line which marks the start of the table
		if strings.Contains(line, "-------") && strings.Contains(line, "------------------") {
			inTable = true
			continue
		}

		if !inTable {
			continue
		}

		// End of table check (usually an empty line or end of matches)
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if len(result) > 0 {
				break // End of results
			}

			continue // Skip leading empty lines in table area
		}

		// Horizontal parsing based on expected widths:
		// ID# (0-7), Name (9-43), Designation (44-55), Alias (57+)
		if len(line) < 10 {
			continue
		}

		id := strings.TrimSpace(safeSubstr(line, 0, 10))
		name := strings.TrimSpace(safeSubstr(line, 10, 35))
		desig := strings.TrimSpace(safeSubstr(line, 45, 12))
		alias := strings.TrimSpace(safeSubstr(line, 57, -1))

		if id == "" {
			continue
		}

		result = append(result, HorizonsResult{
			ID:          id,
			Name:        name,
			Designation: desig,
			Aliases:     strings.Split(alias, "/"),
		})
	}

	err := scanner.Err()
	if err != nil {
		return nil, fmt.Errorf("horizons: scan response: %w", err)
	}

	return result, nil
}

func safeSubstr(s string, start, length int) string {
	if start >= len(s) {
		return ""
	}

	if length == -1 || start+length > len(s) {
		return s[start:]
	}

	return s[start : start+length]
}
