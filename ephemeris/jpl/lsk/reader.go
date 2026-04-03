package lsk

import (
	"bufio"
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

const JPL_LSK_KERNEL_URI = "https://naif.jpl.nasa.gov/pub/naif/generic_kernels/"

type Reader struct {
	F       io.ReadCloser
	DeltaAt []LeapData
}

func Cache(kernel, path string) (*Reader, error) {
	lskPath := filepath.Join(path, kernel)

	if err := os.MkdirAll(filepath.Dir(lskPath), 0755); err != nil {
		return nil, fmt.Errorf("jpl: failed to create parent dir for LSK %s: %w", lskPath, err)
	}

	if _, err := os.Stat(lskPath); os.IsNotExist(err) {
		lskURI := JPL_LSK_KERNEL_URI + kernel
		if err := tools.Download(lskURI, lskPath); err != nil {
			return nil, fmt.Errorf("jpl: failed to download LSK for smallbody: %w", err)
		}
	}

	ls, err := os.Open(lskPath)
	if err != nil {
		return nil, fmt.Errorf("jpl: failed to load LSK %s: %w", lskPath, err)
	}
	return NewReader(ls)
}

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
		if inDeltaAt && strings.Contains(line, ")") {
			inDeltaAt = false
			line = line[:strings.Index(line, ")")]
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
		return nil, fmt.Errorf("jpl: no leapseconds found in LSK")
	}
	l.F = r
	return l, nil
}

func (r *Reader) Close() error {
	return r.F.Close()
}

type LeapData struct {
	JD, N float64
}

func parseSpiceDate(s string) (float64, error) {
	s = strings.TrimPrefix(s, "@")
	parts := strings.Split(s, "-")
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid spice date %s", s)
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
		return 0, fmt.Errorf("invalid month %s", monthStr)
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

func (l *Reader) leapSecondsAt(jdTDB float64) float64 {
	lastN := 0.0
	for _, d := range l.DeltaAt {
		if jdTDB < d.JD {
			break
		}
		lastN = d.N
	}
	return lastN
}

func UTCToTDB(t time.Time, l *Reader) float64 {
	d1, d2 := t.JDParts()
	if t.Scale() == time.TDB {
		return d1 + d2
	}
	jdUTC := d1 + d2
	ls := l.leapSecondsAt(jdUTC + (69.184 / 86400.0))
	return jdUTC + (ls+32.184)/86400.0
}

func TDBToET(jdTDB float64) float64 {
	return (jdTDB - 2451545.0) * 86400.0
}
