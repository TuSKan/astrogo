package jpl

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/body"
	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// KM_PER_AU is the IAU 2012 definition of 1 AU in kilometers.
const KM_PER_AU = 149597870.7

// DE442_URL is the official JPL URL for the DE442 short planetary ephemeris.
const KERNEL_PATH = "https://naif.jpl.nasa.gov/pub/naif/generic_kernels/spk/planets/"

// LSK_URL is a reliable URL for the latest leapseconds kernel.
const LSK_URL = "https://naif.jpl.nasa.gov/pub/naif/generic_kernels/lsk/naif0012.tls"

// Provider implements ephemeris.Provider using JPL SPK/LSK kernels.
type Provider struct {
	DAFReader *Reader
	LSK       *LSK
	Segments  []Segment
}

// New loads a planetary SPK kernel by version (e.g. "de442").
// If the kernel or leapseconds file are not found in ./data, it downloads them.
func New(version string, dataDir string) (*Provider, error) {

	spkPath := filepath.Join(dataDir, version+".bsp")
	lskPath := filepath.Join(dataDir, "naif0012.tls")

	// Ensure files exist
	if _, err := os.Stat(spkPath); os.IsNotExist(err) {
		fmt.Printf("jpl: downloading %s...\n", spkPath)
		if err := Download(KERNEL_PATH+version+".bsp", spkPath); err != nil {
			return nil, fmt.Errorf("jpl: failed to download SPK: %w", err)
		}
	}

	if _, err := os.Stat(lskPath); os.IsNotExist(err) {
		fmt.Printf("jpl: downloading %s...\n", lskPath)
		if err := Download(LSK_URL, lskPath); err != nil {
			return nil, fmt.Errorf("jpl: failed to download LSK: %w", err)
		}
	}

	dr, err := Open(spkPath)
	if err != nil {
		return nil, fmt.Errorf("jpl: failed to open SPK %s: %w", spkPath, err)
	}

	ls, err := loadLSK(lskPath)
	if err != nil {
		dr.Close()
		return nil, fmt.Errorf("jpl: failed to load LSK %s: %w", lskPath, err)
	}

	summaries, err := dr.ReadSummaries()
	if err != nil {
		dr.Close()
		return nil, fmt.Errorf("jpl: failed to read SPK summaries: %w", err)
	}

	var segments []Segment
	for _, s := range summaries {
		segments = append(segments, Segment{
			StartET:   s.Doubles[0],
			EndET:     s.Doubles[1],
			Target:    s.Integers[0],
			Center:    s.Integers[1],
			Frame:     s.Integers[2],
			Type:      s.Integers[3],
			StartAddr: s.Integers[4],
			EndAddr:   s.Integers[5],
		})
	}

	return &Provider{
		DAFReader: dr,
		LSK:       ls,
		Segments:  segments,
	}, nil
}

// Close releases kernel resources.
func (p *Provider) Close() error {
	return p.DAFReader.Close()
}

// State returns the geocentric state (position and velocity) of the given
// body at time t.
func (p *Provider) State(id body.ID, t time.Time) (ephemeris.State, error) {
	naif, ok := BodyIDToNAIF[id]
	if !ok {
		return ephemeris.State{}, fmt.Errorf("jpl: unsupported body ID %v", id)
	}

	tdb := t.TDB()
	jdTDB := tdb.JD()
	et := TDBToET(jdTDB)

	// Get target relative to SSB (0)
	targetSSB, err := p.evaluateRecursive(int32(naif), et, 0)
	if err != nil {
		return ephemeris.State{}, err
	}

	// Get Earth relative to SSB (0)
	earthSSB, err := p.evaluateRecursive(399, et, 0)
	if err != nil {
		return ephemeris.State{}, fmt.Errorf("jpl: failed to get Earth state: %w", err)
	}

	// Geocentric = Target(SSB) - Earth(SSB)
	relPos := targetSSB.Pos.Sub(earthSSB.Pos)
	relVel := targetSSB.Vel.Sub(earthSSB.Vel)

	// Convert to AU and AU/day
	return ephemeris.State{
		Pos: vector.Vec3{
			X: relPos.X / KM_PER_AU,
			Y: relPos.Y / KM_PER_AU,
			Z: relPos.Z / KM_PER_AU,
		},
		Vel: vector.Vec3{
			X: relVel.X * 86400 / KM_PER_AU,
			Y: relVel.Y * 86400 / KM_PER_AU,
			Z: relVel.Z * 86400 / KM_PER_AU,
		},
	}, nil
}

func (p *Provider) evaluateRecursive(targetID int32, et float64, baseID int32) (ephemeris.State, error) {
	currentID := targetID
	var totalPos, totalVel vector.Vec3

	// Limit depth to prevent infinite loops (though SPK trees should be shallow)
	for depth := 0; depth < 10; depth++ {
		if currentID == baseID {
			return ephemeris.State{Pos: totalPos, Vel: totalVel}, nil
		}

		s, err := SelectSegment(p.Segments, currentID, et)
		if err != nil {
			return ephemeris.State{}, err
		}

		pos, vel, err := EvaluateSegment(s, p.DAFReader, et)
		if err != nil {
			return ephemeris.State{}, err
		}

		totalPos = totalPos.Add(pos)
		totalVel = totalVel.Add(vel)
		currentID = s.Center
	}

	return ephemeris.State{}, fmt.Errorf("jpl: recursion depth exceeded for target %d", targetID)
}

// SupportedBodies returns a list of body IDs available in the loaded kernels.
func (p *Provider) SupportedBodies() []body.ID {
	seen := make(map[int32]bool)
	var res []body.ID
	for _, s := range p.Segments {
		if !seen[s.Target] {
			seen[s.Target] = true
			for bid, naif := range BodyIDToNAIF {
				if int32(naif) == s.Target {
					res = append(res, bid)
				}
			}
		}
	}
	return res
}

// --- LSK Layer ---

type LSK struct {
	DeltaAt []LeapData
}

type LeapData struct {
	JD, N float64
}

func loadLSK(path string) (*LSK, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	l := &LSK{}
	scanner := bufio.NewScanner(f)
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
		return nil, fmt.Errorf("jpl: no leapseconds found in LSK %s", path)
	}
	return l, nil
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

func (l *LSK) leapSecondsAt(jdTDB float64) float64 {
	lastN := 0.0
	for _, d := range l.DeltaAt {
		if jdTDB < d.JD {
			break
		}
		lastN = d.N
	}
	return lastN
}

func UTCToTDB(t time.Time, l *LSK) float64 {
	d1, d2 := t.JDParts()
	jdUTC := d1 + d2
	ls := l.leapSecondsAt(jdUTC + (69.184 / 86400.0))
	return jdUTC + (ls+32.184)/86400.0
}

func TDBToET(jdTDB float64) float64 {
	return (jdTDB - 2451545.0) * 86400.0
}

// --- Bodies Layer ---

var BodyIDToNAIF = map[body.ID]int{
	body.Sun: 10, body.Moon: 301, body.Mercury: 199, body.Venus: 299,
	body.Earth: 399, body.Mars: 4, body.Jupiter: 5, body.Saturn: 6,
	body.Uranus: 7, body.Neptune: 8,
}
