package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Mapping from OpenNGC types to astrogo Kind
func mapType(t string) string { // Changed return type to string to match targetRecord.Kind
	switch t {
	case "G", "Gx", "Gxy", "G_Ctr", "GClstr":
		return "Galaxy"
	case "Nb", "HII", "PN", "SNR", "RfN", "Neb", "DrkN": // Added Neb, DrkN from old map
		return "Nebula"
	case "OCl", "Cl", "Cl+N", "Assoc", "NAssoc", "OCl+N":
		return "OpenCluster"
	case "GCl":
		return "GlobularCluster"
	case "*", "**", "*Assoc", "Star": // Added *Assoc from old map
		return "Star"
	case "Ast":
		return "Asterism"
	case "GGroup", "GPair", "GTrpl": // Added from old map
		return "Galaxy"
	default:
		return "Other"
	}
}

func main() {
	if len(os.Args) < 3 {
		log.Fatal("Usage: go run parse.go <output_path> <url1> [url2...]")
	}

	outputPath := os.Args[1]
	urls := os.Args[2:]

	var records []targetRecord

	for _, url := range urls {
		fileRecords, err := downloadAndParse(url)
		if err != nil {
			log.Fatalf("error processing %s: %v", url, err)
		}
		records = append(records, fileRecords...)
	}

	// Sort by ID for stability
	sort.Slice(records, func(i, j int) bool {
		return records[i].ID < records[j].ID
	})

	if err := writeRuntimeCSV(outputPath, records); err != nil {
		log.Fatalf("error writing output: %v", err)
	}

	fmt.Printf("Successfully generated %d records to %s\n", len(records), outputPath)
}

type targetRecord struct {
	ID      string
	Name    string
	Kind    string
	RA      float64
	Dec     float64
	Aliases []string
	VMag    string // V-band apparent magnitude (empty if unavailable)
	BMag    string // B-band apparent magnitude (empty if unavailable)
}

func downloadAndParse(url string) ([]targetRecord, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status: %s", resp.Status)
	}

	return parseOpenNGC(resp.Body)
}

func parseOpenNGC(input io.Reader) ([]targetRecord, error) {
	r := csv.NewReader(input)
	r.Comma = ';'
	r.LazyQuotes = true

	header, err := r.Read()
	if err != nil {
		return nil, err
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
			return nil, err
		}

		kindStr := row[col["Type"]]
		mappedKind := mapType(kindStr)
		if mappedKind == "Other" {
			continue
		}

		name := row[col["Name"]]
		raStr := row[col["RA"]]
		decStr := row[col["Dec"]]
		mID := row[col["M"]]
		commonNames := row[col["Common names"]]
		identifiers := row[col["Identifiers"]]

		// Some entries are just cross-references or have empty coords
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

		// Pick first common name as Name for better UI
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
			Kind:    mappedKind,
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
// Returns empty string if column doesn't exist or value is empty.
func extractMag(row []string, col map[string]int, colName string) string {
	idx, ok := col[colName]
	if !ok || idx >= len(row) {
		return ""
	}
	v := strings.TrimSpace(row[idx])
	if v == "" {
		return ""
	}
	// Validate it's a parseable number.
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
		// OpenNGC M field is just the number sometimes, or "Mxx"
		mNum := strings.TrimPrefix(mID, "M")
		mNum = strings.TrimLeft(mNum, "0")
		if mNum != "" {
			add("M " + mNum)
			add("M" + mNum)
			add("Messier " + mNum)
		}
	}

	for _, a := range strings.Split(commonNames, ",") {
		add(a)
	}
	for _, a := range strings.Split(identifiers, ",") {
		add(a)
	}

	return aliases
}

func parseRA(s string) (float64, error) {
	// HH:MM:SS.SS
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid RA format")
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
		return 0, fmt.Errorf("invalid Dec format")
	}
	d, _ := strconv.ParseFloat(parts[0], 64)
	m, _ := strconv.ParseFloat(parts[1], 64)
	sVal, _ := strconv.ParseFloat(parts[2], 64)
	return sign * (d + m/60 + sVal/3600), nil
}

func writeRuntimeCSV(path string, records []targetRecord) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	// Output format: id,name,kind,ra_deg,dec_deg,aliases(semicolon separated),vmag,bmag
	if err := w.Write([]string{"id", "name", "kind", "ra", "dec", "aliases", "vmag", "bmag"}); err != nil {
		return err
	}

	for _, rec := range records {
		if err := w.Write([]string{
			rec.ID,
			rec.Name,
			rec.Kind,
			fmt.Sprintf("%.6f", rec.RA),
			fmt.Sprintf("%.6f", rec.Dec),
			strings.Join(rec.Aliases, ";"),
			rec.VMag,
			rec.BMag,
		}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}
