package iers

import (
	"bufio"
	"bytes"
	"io"
	"strconv"
	"strings"
	_ "embed"
)

//go:generate go run ../internal/tools/download.go https://datacenter.iers.org/products/eop/rapid/standard/finals2000A.data data/finals2000A.data
//go:embed data/finals2000A.data
var iersData []byte

var globalOffsets map[int]float64

func init() {
	// Execute the structured mapping extracting strictly accurate properties synchronously during module instantiation.
	globalOffsets, _ = ParseIERS(bytes.NewReader(iersData))
}

// ParseIERS extracts UT1-UTC offsets dynamically bounded to mathematically integer 
// Modified Julian Dates parsing strict fixed-width structural sequences natively.
func ParseIERS(reader io.Reader) (map[int]float64, error) {
	result := make(map[int]float64)
	scanner := bufio.NewScanner(reader)

	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 68 {
			continue // Avoid slicing out-of-bounds strings fundamentally
		}

		mjdStr := strings.TrimSpace(line[7:15])
		mjdFloat, err := strconv.ParseFloat(mjdStr, 64)
		if err != nil {
			continue 
		}
		
		mjd := int(mjdFloat)

		offsetStr := strings.TrimSpace(line[58:68])
		offset, err := strconv.ParseFloat(offsetStr, 64)
		if err != nil {
			continue 
		}

		result[mjd] = offset
	}

	if err := scanner.Err(); err != nil {
		return result, err
	}

	return result, nil
}

// GetOffset provides purely functional structural parsing extracting cached UT1-UTC mappings natively.
// Fallback guarantees a 0.0 geometric baseline preventing nil boundary calculations.
func GetOffset(mjd int) float64 {
	if globalOffsets == nil {
		return 0.0
	}
	
	offset, exists := globalOffsets[mjd]
	if !exists {
		return 0.0
	}
	
	return offset
}
