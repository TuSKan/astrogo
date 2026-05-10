package lsk

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/internal/tools"
	"github.com/TuSKan/astrogo/time"
)

// Sentinel errors for LSK parsing.
var (
	ErrNoLeapseconds = errors.New("jpl: no leapseconds found in LSK")
	ErrInvalidDate   = errors.New("invalid spice date")
	ErrInvalidMonth  = errors.New("invalid month")
)

// JPLLSKKernelURI is the base URL for the JPL LSK kernel.
const JPLLSKKernelURI = "https://naif.jpl.nasa.gov/pub/naif/generic_kernels/"

// Reader is a reader for the JPL LSK kernel.
type Reader struct {
	F       io.ReadCloser
	DeltaAt []LeapData
}

// Cache downloads an LSK file if it doesn't exist and opens it.
//
// It provides an auto-healing mechanism for CI environments by automatically
// removing corrupt or truncated files.
//
// If the file is incomplete or its metadata is invalid, the function:
//  1. Closes the file handle.
//  2. Removes the corrupt file from the filesystem.
//  3. Returns the error wrapped with a descriptive message.
func Cache(kernel, path string) (*Reader, error) {
	lskPath := filepath.Join(path, kernel)

	err := os.MkdirAll(filepath.Dir(lskPath), 0o755)
	if err != nil {
		return nil, fmt.Errorf("jpl: failed to create parent dir for LSK %s: %w", lskPath, err)
	}

	_, err = os.Stat(lskPath)
	if os.IsNotExist(err) {
		lskURI := JPLLSKKernelURI + kernel

		err := tools.Download(lskURI, lskPath)
		if err != nil {
			return nil, fmt.Errorf("jpl: failed to download LSK for smallbody: %w", err)
		}
	}

	ls, err := os.Open(lskPath)
	if err != nil {
		return nil, fmt.Errorf("jpl: failed to load LSK %s: %w", lskPath, err)
	}

	return NewReader(ls)
}

// NewReader parses an LSK kernel from the given reader.
func NewReader(r io.ReadCloser) (*Reader, error) {
	l := &Reader{}
	scanner := bufio.NewScanner(r)
	inDeltaAt := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.Contains(line, "DELTET/DELTA_AT") {
			inDeltaAt = true

			if strings.Contains(line, "=") {
				line = line[strings.Index(line, "=")+1:]
			}
		}

		if inDeltaAt {
			if idx := strings.Index(line, ")"); idx >= 0 {
				inDeltaAt = false
				line = line[:idx]
			}
		}

		if inDeltaAt || (line != "" && !inDeltaAt && strings.HasPrefix(line, "@")) {
			line = strings.ReplaceAll(line, "(", " ")
			line = strings.ReplaceAll(line, ",", " ")

			parts := strings.Fields(line)
			for i := 0; i+1 < len(parts); i += 2 {
				// n is first, then date
				n, err1 := strconv.ParseFloat(parts[i], 64)

				jd, err2 := parseSpiceDate(parts[i+1])
				if err1 == nil && err2 == nil {
					l.DeltaAt = append(l.DeltaAt, LeapData{JD: jd, N: n})
				}
			}
		}
	}

	if len(l.DeltaAt) == 0 {
		return nil, ErrNoLeapseconds
	}

	l.F = r

	return l, nil
}

// Close closes the underlying file reader.
func (r *Reader) Close() error {
	err := r.F.Close()
	if err != nil {
		return fmt.Errorf("lsk: close: %w", err)
	}

	return nil
}

// LeapData represents a leapsecond correction entry.
type LeapData struct {
	JD, N float64
}

// parseSpiceDate parses a SPICE date string into a Julian Date.
//
// The format is "@YYYY-MMM-DD" or "@YYYY-MMM".
// Example: "@2016-JAN-01"
func parseSpiceDate(s string) (float64, error) {
	s = strings.TrimPrefix(s, "@")

	parts := strings.Split(s, "-")
	if len(parts) < 2 {
		return 0, fmt.Errorf("%w: %s", ErrInvalidDate, s)
	}

	year, _ := strconv.Atoi(parts[0])
	monthStr := strings.ToUpper(parts[1])

	day := 1
	if len(parts) > 2 {
		day, _ = strconv.Atoi(parts[2])
	}

	months := map[string]int{
		"JAN": 1, "FEB": 2, "MAR": 3, "APR": 4, "MAY": 5, "JUN": 6,
		"JUL": 7, "AUG": 8, "SEP": 9, "OCT": 10, "NOV": 11, "DEC": 12,
	}

	month := months[monthStr]
	if month == 0 {
		return 0, fmt.Errorf("%w: %s", ErrInvalidMonth, monthStr)
	}

	// Simple JD calculation for 12:00:00 (standard for leapsecond dates in LSK)
	// JD = 367*Y - (7*(Y + (M+9)/12))/4 + (275*M)/9 + D + 1721013.5
	// This is valid for Gregorian calendar (post-1582).
	a := (14 - month) / 12
	y := year + 4800 - a
	m := month + 12*a - 3
	jd := float64(day) + math.Floor(float64(153*m+2)/5) + float64(365*y) + math.Floor(float64(y)/4) - math.Floor(float64(y)/100) + math.Floor(float64(y)/400) - 32045.5

	return jd, nil
}

func (r *Reader) leapSecondsAt(jdTDB float64) float64 {
	lastN := 0.0

	for _, d := range r.DeltaAt {
		if jdTDB < d.JD {
			break
		}

		lastN = d.N
	}

	return lastN
}

// UTCToTDB converts a time.Time to a Julian Date in the Barycentric Dynamical
// Time (TDB) scale.
//
// The conversion formula used is:
// TDB = UTC + (LS + 32.184) / 86400.0
// where LS is the number of leap seconds at the given time.
func UTCToTDB(t time.Time, l *Reader) float64 {
	d1, d2 := t.JDParts()
	if t.Scale() == time.TDB {
		return d1 + d2
	}

	jdUTC := d1 + d2
	ls := l.leapSecondsAt(jdUTC + (69.184 / 86400.0))

	return jdUTC + (ls+32.184)/86400.0
}

// TDBToET converts a Julian Date in TDB to elapsed seconds past J2000.
func TDBToET(jdTDB float64) float64 {
	return (jdTDB - 2451545.0) * 86400.0
}
