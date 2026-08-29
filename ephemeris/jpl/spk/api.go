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
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/remote/api"
	"github.com/TuSKan/astrogo/remote/file"
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

	// Error is Horizons' own explanation when it declines a request.
	//
	// It was not parsed at all, so a refusal arrived as an empty kernel
	// list and a nil error, and the caller was left to guess. Horizons is
	// specific when it declines — for 101955 Bennu it answers "SPK creation
	// is not available for pre-computed objects in the major body index" —
	// and that sentence is worth more than anything this package can infer.
	Error string `json:"error"`
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

// CacheAPI returns readers for kernel, generating it through JPL Horizons
// and storing it under prefix in bucket when it is not already there.
// Horizons delivers a small-body kernel base64-encoded inside its JSON
// response, so this is a real download and is gated by
// remote.JPLHorizonsSPK's consent exactly like any other.
//
// Storage is a bucket and key prefix, never a directory path: a generated
// kernel lands wherever remote's cache lives, which need not be local
// disk.
func CacheAPI(ctx context.Context, bucket *file.Bucket, prefix, kernel string, startTime, endTime time.Time) ([]*Reader, error) {
	var readers []*Reader

	spkFile := prefix + kernel + ".bsp"

	// A failed existence check just falls through to the fetch path below,
	// same as a real miss.
	if exists, _ := bucket.Exists(ctx, spkFile); exists {
		reader, err := openKernel(ctx, bucket, spkFile)
		if err != nil {
			return nil, err
		}

		return []*Reader{reader}, nil
	}

	// Generating and storing a small-body SPK kernel via Horizons is a file
	// download in effect (KB–MB delivered base64-encoded inside the JSON
	// response), so it requires the same explicit consent as any other
	// kernel download: remote.EnableDownloads(maxSize, remote.JPLHorizonsSPK).
	if err := remote.CheckDownload(remote.JPLHorizonsSPK, spkFile, remote.SizeVaries); err != nil {
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
	var (
		resp    *HorizonsResponse
		refusal string
	)

	for _, command := range commandCandidates(kernel) {
		r, err := apiHorizonsRequest(ctx, command, startTime, endTime)
		if err != nil {
			return nil, fmt.Errorf("jpl: failed to get SPK %s: %w", kernel, err)
		}

		resp = r
		if resp.SpkFileID != "" && resp.Spk != "" {
			break
		}

		if r.Error != "" {
			refusal = r.Error
		}
	}

	// Horizons answered, and declined. Say why it declined rather than
	// returning an empty list with a nil error, which reads as success and
	// surfaces much later as a body that is simply missing.
	if refusal != "" && (resp == nil || resp.Spk == "") {
		return nil, fmt.Errorf("%w: %s (kernel %q)", ErrHorizonsRefused, collapseWhitespace(refusal), kernel)
	}

	if resp.SpkFileID != "" && resp.Spk != "" {
		spkFile = prefix + resp.SpkFileID + ".bsp"

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

		if err := file.Save(ctx, bucket, spkFile, bytes.NewReader(spkData)); err != nil {
			return nil, fmt.Errorf("jpl: failed to save SPK %s: %w", spkFile, err)
		}

		reader, err := openKernel(ctx, bucket, spkFile)
		if err != nil {
			return nil, err
		}

		// A freshly Horizons-generated kernel is validated before being
		// trusted/cached: live-confirmed this session that Horizons can
		// return a DAF file record whose FWARD claims a first summary
		// record exists (FWD != 0) while that record — and everything
		// after the comment area — is entirely zeroed, so ReadSummaries
		// finds nothing despite the file record's own claim. That specific
		// contradiction (FWD != 0 but zero summaries actually read) is
		// what's checked here, not "zero summaries" alone — a kernel that
		// honestly declares FWD == 0 (no summary records at all) is a
		// self-consistent, legitimately empty file, not corrupt (this is
		// exactly the minimal fixture reader_test.go/api_test.go's own
		// synthetic DAF headers use, and must keep working unchanged).
		// Caching the inconsistent case would silently and permanently
		// break every future lookup for kernel, since CacheAPI's own
		// existence check would keep reusing it — delete it and surface a
		// clear error instead.
		if reader.FileRec.FWD != 0 {
			summaries, err := reader.ReadSummaries()
			if err != nil {
				_ = reader.Close()
				_ = bucket.Delete(ctx, spkFile)

				return nil, fmt.Errorf("jpl: validate SPK %s: %w", spkFile, err)
			}

			if len(summaries) == 0 {
				_ = reader.Close()
				_ = bucket.Delete(ctx, spkFile)

				return nil, fmt.Errorf("%w: %s (kernel %s)", ErrHorizonsEmptyKernel, spkFile, kernel)
			}
		}

		readers = append(readers, reader)
	} else {
		hRes, err := parseHorizonsResult(resp.Result)
		if err != nil {
			return nil, fmt.Errorf("jpl: failed to parse Horizons result: %w", err)
		}

		for _, r := range hRes {
			sub, err := CacheAPI(ctx, bucket, prefix, r.ID, startTime, endTime)
			if err != nil {
				return nil, fmt.Errorf("jpl: failed to get SPK %s: %w", kernel, err)
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

	client, err := api.NewClient(remote.JPLHorizonsSPK)
	if err != nil {
		return nil, fmt.Errorf("jpl: horizons client: %w", err)
	}

	var resp HorizonsResponse
	if err := client.GetJSON(ctx, remote.JPLHorizonsSPK, "", params, &resp); err != nil {
		return nil, mapHorizonsStatus(err)
	}

	return &resp, nil
}

// mapHorizonsStatus translates remote's typed HTTP errors into this
// package's documented Horizons sentinels. Non-HTTP errors (registry gate,
// network failures) pass through wrapped.
func mapHorizonsStatus(err error) error {
	var httpErr *api.HTTPError
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

// openKernel builds a Reader over bucket/key with random access served by
// remote/file's chunk-caching reader — the same path for a local cache, an
// S3 bucket, or anything else a driver serves.
func openKernel(ctx context.Context, bucket *file.Bucket, key string) (*Reader, error) {
	ra, err := file.NewReaderAt(ctx, bucket, key)
	if err != nil {
		return nil, fmt.Errorf("jpl: open SPK %s: %w", key, err)
	}

	reader, err := NewReader(ra)
	if err != nil {
		return nil, fmt.Errorf("jpl: failed to create reader for %s: %w", key, err)
	}

	return reader, nil
}

// collapseWhitespace folds Horizons' multi-line refusal onto one line.
//
// The service formats these for a terminal, with embedded newlines and runs
// of spaces; an error string carrying those wraps badly in a log and reads as
// several failures rather than one.
func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
