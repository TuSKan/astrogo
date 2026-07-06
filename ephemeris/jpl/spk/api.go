package spk

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	stdtime "time"

	"github.com/TuSKan/astrogo/time"
)

// DOC: https://ssd-api.jpl.nasa.gov/doc/horizons.html
// API: https://ssd-api.jpl.nasa.gov/horizons.api
// SELECT: https://ssd.jpl.nasa.gov/horizons/manual.html#select

// JPLHorizonsAPI is the base URL for the JPL Horizons API.
const JPLHorizonsAPI = "https://ssd.jpl.nasa.gov/api/horizons.api"

// horizonsRequestTimeout bounds the whole Horizons API request (connect +
// transfer), preventing an indefinite hang on a stalled connection.
const horizonsRequestTimeout = 2 * stdtime.Minute

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

// CacheAPI caches an SPK file from JPL Horizons if it doesn't exist.
//
// It automatically handles:
// - Directory creation
// - File existence check
// - Base64 decoding
// - File writing
// - Reader creation
// - Error handling
func CacheAPI(kernel string, startTime, endTime time.Time, path string) ([]*Reader, error) {
	var readers []*Reader

	spkFile := kernel + ".bsp"
	spkPath := filepath.Join(path, spkFile)

	if err := os.MkdirAll(path, 0o755); err != nil {
		return nil, fmt.Errorf("jpl: failed to create directory %s: %w", path, err)
	}

	if _, err := os.Stat(spkPath); err == nil {
		// Already exists, just return reader
		f, err := os.Open(spkPath)
		if err != nil {
			return nil, fmt.Errorf("jpl: failed to open cached SPK %s: %w", spkPath, err)
		}

		reader, err := NewReader(f)
		if err != nil {
			return nil, fmt.Errorf("jpl: failed to create reader for %s: %w", spkPath, err)
		}

		return []*Reader{reader}, nil
	}

	resp, err := apiHorizonsRequest(kernel, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("jpl: failed to get SPK %s: %w", spkPath, err)
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

		if err := os.WriteFile(spkPath, spkData, 0o644); err != nil {
			return nil, fmt.Errorf("jpl: failed to save SPK %s: %w", spkPath, err)
		}

		f, err := os.Open(spkPath)
		if err != nil {
			return nil, fmt.Errorf("jpl: failed to open SPK %s: %w", spkPath, err)
		}

		reader, err := NewReader(f)
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
			sub, err := CacheAPI(r.ID, startTime, endTime, path)
			if err != nil {
				return nil, fmt.Errorf("jpl: failed to get SPK %s: %w", spkPath, err)
			}

			readers = append(readers, sub...)
		}
	}

	return readers, nil
}

func apiHorizonsRequest(command string, startTime, endTime time.Time) (_ *HorizonsResponse, err error) {
	api, err := url.Parse(JPLHorizonsAPI)
	if err != nil {
		return nil, fmt.Errorf("jpl: failed to parse API URL: %w", err)
	}

	params := url.Values{}
	params.Set("format", "json")
	params.Set("COMMAND", "'"+command+"'")
	params.Set("MAKE_EPHEM", "YES")
	params.Set("EPHEM_TYPE", "SPK")
	params.Set("OBJ_DATA", "NO")
	params.Set("START_TIME", "'"+startTime.Format("2006-01-02 15:04:05.000")+"'")
	params.Set("STOP_TIME", "'"+endTime.Format("2006-01-02 15:04:05.000")+"'")

	api.RawQuery = params.Encode()

	ctx, cancel := context.WithTimeout(context.Background(), horizonsRequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("HorizonsRequest: failed to create request: %w", err)
	}

	r, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HorizonsRequest: failed to get: %w", err)
	}
	defer func() {
		cerr := r.Body.Close()
		if cerr != nil {
			err = errors.Join(err, cerr)
		}
	}()

	switch r.StatusCode {
	case http.StatusOK:
		// Proceed
	case http.StatusBadRequest:
		return nil, ErrHorizonsBadRequest
	case http.StatusMethodNotAllowed:
		return nil, ErrHorizonsMethodNA
	case http.StatusInternalServerError:
		return nil, ErrHorizonsServerError
	case http.StatusServiceUnavailable:
		return nil, ErrHorizonsUnavailable
	default:
		return nil, fmt.Errorf("%w: %s", ErrHorizonsUnexpected, r.Status)
	}

	var resp HorizonsResponse
	if err := json.NewDecoder(r.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("jpl: failed to decode response: %w", err)
	}

	return &resp, nil
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
