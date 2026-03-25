package catalog

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	csv "github.com/nnnkkk7/go-simdcsv"
)

type DeepSkyTarget struct {
	ID            string
	CommonNames   string
	Identifiers   string
	Messier       string
	Type          string
	Const         string
	RA            float64 // Stored in Radians
	Dec           float64 // Stored in Radians
	MajorAxis     float64
	MinorAxis     float64
	VMag          float64
	SurfaceBright float64
}

// parseRA transforms a HH:MM:SS.s Right Ascension string into a numeric radian mapping.
// Returns math.NaN() if parsing fails.
func parseRA(raStr string) float64 {
	raStr = strings.TrimSpace(raStr)
	if raStr == "" {
		return math.NaN()
	}

	parts := strings.Split(raStr, ":")
	var h, m, s float64
	var err error

	if len(parts) > 0 {
		h, err = strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return math.NaN()
		}
	}
	if len(parts) > 1 {
		m, err = strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return math.NaN()
		}
	}
	if len(parts) > 2 {
		s, err = strconv.ParseFloat(parts[2], 64)
		if err != nil {
			return math.NaN()
		}
	}

	hours := h + (m / 60.0) + (s / 3600.0)
	return hours * (math.Pi / 12.0)
}

// parseDec translates a ±DD:MM:SS.s Declination string into a numeric radian mapping.
// Properly tracks signed zeros like -00:15:30. Returns math.NaN() if parsing fails.
func parseDec(decStr string) float64 {
	decStr = strings.TrimSpace(decStr)
	if decStr == "" {
		return math.NaN()
	}

	sign := 1.0
	if strings.HasPrefix(decStr, "-") {
		sign = -1.0
		decStr = strings.TrimPrefix(decStr, "-")
	} else if strings.HasPrefix(decStr, "+") {
		decStr = strings.TrimPrefix(decStr, "+")
	}

	parts := strings.Split(decStr, ":")
	var d, m, s float64
	var err error

	if len(parts) > 0 {
		d, err = strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return math.NaN()
		}
	}
	if len(parts) > 1 {
		m, err = strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return math.NaN()
		}
	}
	if len(parts) > 2 {
		s, err = strconv.ParseFloat(parts[2], 64)
		if err != nil {
			return math.NaN()
		}
	}

	degrees := d + (m / 60.0) + (s / 3600.0)
	return sign * degrees * (math.Pi / 180.0)
}

// parseFloatField securely parses a numeric standard formatting.
// Resolves safely to math.NaN() upon emptiness.
func parseFloatField(val string) float64 {
	val = strings.TrimSpace(val)
	if val == "" {
		return math.NaN()
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return math.NaN()
	}
	return f
}

// ParseNGC constructs an efficient map from standard OpenNGC semantic boundaries.
func ParseNGC(reader io.Reader) ([]DeepSkyTarget, error) {
	r := csv.NewReader(reader)
	r.Comma = ';'
	r.LazyQuotes = true
	r.FieldsPerRecord = -1

	// Using ReadAll simplifies the approach, efficiently loading everything into memory.
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}

	if len(records) < 2 {
		return []DeepSkyTarget{}, nil
	}

	header := records[0]
	idxMap := make(map[string]int)
	for i, col := range header {
		idxMap[strings.TrimSpace(col)] = i
	}

	// Validate presence of baseline crucial variables
	requiredCols := []string{"Name", "Type", "RA", "Dec", "Const"}
	for _, req := range requiredCols {
		if _, exists := idxMap[req]; !exists {
			return nil, fmt.Errorf("missing required column: %s", req)
		}
	}

	targets := make([]DeepSkyTarget, 0, len(records)-1)

	for _, record := range records[1:] {
		getField := func(name string) string {
			idx, exists := idxMap[name]
			if !exists || idx >= len(record) {
				return ""
			}
			return strings.TrimSpace(record[idx])
		}

		name := getField("Name")
		if name == "" {
			continue // Skip void configurations natively.
		}

		target := DeepSkyTarget{
			ID:            name,
			CommonNames:   getField("Common names"),
			Identifiers:   getField("Identifiers"),
			Messier:       getField("M"),
			Type:          getField("Type"),
			Const:         getField("Const"),
			RA:            parseRA(getField("RA")),
			Dec:           parseDec(getField("Dec")),
			MajorAxis:     parseFloatField(getField("MajAx")),
			MinorAxis:     parseFloatField(getField("MinAx")),
			VMag:          parseFloatField(getField("V-Mag")),
			SurfaceBright: parseFloatField(getField("SurfBr")),
		}

		targets = append(targets, target)
	}

	return targets, nil
}
