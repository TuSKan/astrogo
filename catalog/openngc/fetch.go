package openngc

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

// Sentinel errors for the raw OpenNGC CSV format.
var (
	errInvalidRA  = errors.New("openngc: invalid RA format")
	errInvalidDec = errors.New("openngc: invalid Dec format")
)

// fetch downloads the OpenNGC source CSVs, merges them, and returns the
// result as resolve.Targets — called by New() on every construction. The
// set of source files comes from the remote.OpenNGC endpoint's own
// registered Files manifest (the registry, not this package, owns which
// files the endpoint serves). Each source file goes through fetchSource: a
// HEAD probe reuses the on-disk cache if the upstream content hasn't
// changed (not a wall-clock expiration window), so calling New()
// repeatedly costs a full download only the first time (or whenever the
// upstream files actually change).
//
// Returns remote.ErrDownloadDenied unless
// remote.EnableDownloads(remote.OpenNGC, maxSize) has been called; it
// still respects remote.SetOffline and remote.Disable(remote.OpenNGC).
func fetch(ctx context.Context) ([]resolve.Target, error) {
	ep, ok := remote.Lookup(remote.OpenNGC)
	if !ok {
		return nil, fmt.Errorf("%w: %q", remote.ErrUnknownEndpoint, remote.OpenNGC)
	}

	var records []targetRecord

	for _, sourceFile := range ep.Files {
		recs, err := fetchSource(ctx, sourceFile)
		if err != nil {
			return nil, err
		}

		records = append(records, recs...)
	}

	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })

	return toTargets(records), nil
}

// fetchSource downloads and parses one OpenNGC source CSV, reusing the
// on-disk cache when remote.GetFile's HEAD probe shows it's still current.
func fetchSource(ctx context.Context, sourceFile string) ([]targetRecord, error) {
	f, err := remote.GetFile(ctx, remote.OpenNGC, sourceFile)
	if err != nil {
		return nil, fmt.Errorf("openngc: %s: %w", sourceFile, err)
	}

	r, err := f.OpenReader()
	if err != nil {
		return nil, fmt.Errorf("openngc: open %s: %w", sourceFile, err)
	}

	defer r.Close() //nolint:errcheck // read-only handle, nothing actionable on close failure

	recs, err := parseOpenNGC(r)
	if err != nil {
		return nil, fmt.Errorf("openngc: parse %s: %w", sourceFile, err)
	}

	return recs, nil
}

// targetRecord is one parsed row of the raw upstream OpenNGC CSV format.
type targetRecord struct {
	ID      string
	Name    string
	Kind    resolve.Kind
	VMag    string
	BMag    string
	Aliases []string
	RA      float64
	Dec     float64
}

// toTargets converts merged, sorted targetRecords directly into
// resolve.Targets.
func toTargets(records []targetRecord) []resolve.Target {
	targets := make([]resolve.Target, 0, len(records))

	for _, rec := range records {
		t := resolve.Target{
			ID:       rec.ID,
			Name:     rec.Name,
			Kind:     rec.Kind,
			Coord:    coord.NewICRS(angle.Deg(rec.RA), angle.Deg(rec.Dec)),
			HasCoord: true,
			Catalog:  "openngc",
			Aliases:  rec.Aliases,
			Epoch:    time.J2000, // OpenNGC's RA/Dec are J2000 by catalog convention
		}

		if v, err := strconv.ParseFloat(rec.VMag, 64); err == nil {
			t.VMag = v
			t.HasVMag = true
		}

		targets = append(targets, t)
	}

	return targets
}

// mapKind maps an OpenNGC object-type code to astrogo's resolve.Kind.
func mapKind(t string) resolve.Kind {
	switch t {
	case "G", "Gx", "Gxy", "G_Ctr", "GClstr":
		return resolve.KindGalaxy
	case "Nb", "HII", "PN", "SNR", "RfN", "Neb", "DrkN":
		return resolve.KindNebula
	case "OCl", "Cl", "Cl+N", "Assoc", "NAssoc", "OCl+N":
		return resolve.KindOpenCluster
	case "GCl":
		return resolve.KindGlobularCluster
	case "*", "**", "*Assoc", "Star":
		return resolve.KindStar
	case "Ast":
		return resolve.KindAsterism
	case "GGroup", "GPair", "GTrpl":
		return resolve.KindGalaxy
	default:
		return resolve.KindOther
	}
}

// parseOpenNGC parses the raw, semicolon-delimited upstream OpenNGC CSV
// format (NGC.csv / addendum.csv) into targetRecords.
func parseOpenNGC(input io.Reader) ([]targetRecord, error) {
	r := csv.NewReader(input)
	r.Comma = ';'
	r.LazyQuotes = true

	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	col := make(map[string]int)
	for i, h := range header {
		col[h] = i
	}

	var records []targetRecord

	for {
		row, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("read row: %w", err)
		}

		kindStr := row[col["Type"]]

		kind := mapKind(kindStr)
		if kind == resolve.KindOther {
			continue
		}

		name := row[col["Name"]]
		raStr := row[col["RA"]]
		decStr := row[col["Dec"]]
		mID := row[col["M"]]
		commonNames := row[col["Common names"]]
		identifiers := row[col["Identifiers"]]

		// Some entries are just cross-references or have empty coords.
		if raStr == "" || decStr == "" {
			continue
		}

		ra, err := parseRA(raStr)
		if err != nil {
			continue
		}

		dec, err := parseDec(decStr)
		if err != nil {
			continue
		}

		id := normalizeID(name)
		aliases := gatherAliases(id, mID, commonNames, identifiers)

		// Pick first common name as Name for better UI.
		displayName := name

		if commonNames != "" {
			first := strings.TrimSpace(strings.Split(commonNames, ",")[0])
			if first != "" {
				displayName = first
			}
		}

		records = append(records, targetRecord{
			ID:      id,
			Name:    displayName,
			Kind:    kind,
			RA:      ra,
			Dec:     dec,
			Aliases: aliases,
			VMag:    extractMag(row, col, "V-Mag"),
			BMag:    extractMag(row, col, "B-Mag"),
		})
	}

	return records, nil
}

// extractMag safely extracts a magnitude value from a row by column name.
// Returns an empty string if the column doesn't exist or is empty/invalid.
func extractMag(row []string, col map[string]int, colName string) string {
	idx, ok := col[colName]
	if !ok || idx >= len(row) {
		return ""
	}

	v := strings.TrimSpace(row[idx])
	if v == "" {
		return ""
	}

	if _, err := strconv.ParseFloat(v, 64); err != nil {
		return ""
	}

	return v
}

func normalizeID(s string) string {
	s = strings.ReplaceAll(s, " ", "")

	s = strings.ToUpper(s)
	if strings.HasPrefix(s, "NGC") {
		return "NGC" + strings.TrimLeft(s[3:], "0")
	}

	if strings.HasPrefix(s, "IC") {
		return "IC" + strings.TrimLeft(s[2:], "0")
	}

	return s
}

func gatherAliases(id, mID, commonNames, identifiers string) []string {
	seen := make(map[string]bool)
	seen[strings.ToLower(id)] = true

	var aliases []string

	add := func(a string) {
		a = strings.TrimSpace(a)
		if a == "" {
			return
		}

		norm := strings.ToLower(strings.ReplaceAll(a, " ", ""))
		if !seen[norm] {
			seen[norm] = true

			aliases = append(aliases, a)
		}
	}

	if mID != "" {
		// OpenNGC's M field is just the number sometimes, or "Mxx".
		mNum := strings.TrimPrefix(mID, "M")

		mNum = strings.TrimLeft(mNum, "0")
		if mNum != "" {
			add("M " + mNum)
			add("M" + mNum)
			add("Messier " + mNum)
		}
	}

	for a := range strings.SplitSeq(commonNames, ",") {
		add(a)
	}

	for a := range strings.SplitSeq(identifiers, ",") {
		add(a)
	}

	return aliases
}

func parseRA(s string) (float64, error) {
	// HH:MM:SS.SS
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, errInvalidRA
	}

	h, _ := strconv.ParseFloat(parts[0], 64)
	m, _ := strconv.ParseFloat(parts[1], 64)
	sVal, _ := strconv.ParseFloat(parts[2], 64)

	return h*15 + m/4 + sVal/240, nil
}

func parseDec(s string) (float64, error) {
	// +DD:MM:SS.S
	sign := 1.0
	if strings.HasPrefix(s, "-") {
		sign = -1.0
		s = s[1:]
	} else if strings.HasPrefix(s, "+") {
		s = s[1:]
	}

	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, errInvalidDec
	}

	d, _ := strconv.ParseFloat(parts[0], 64)
	m, _ := strconv.ParseFloat(parts[1], 64)
	sVal, _ := strconv.ParseFloat(parts[2], 64)

	return sign * (d + m/60 + sVal/3600), nil
}
