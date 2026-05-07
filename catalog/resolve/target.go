package resolve

import (
	"errors"
	"strings"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// Kind represents the type of an astronomical object.
type Kind string

const (
	KindStar             Kind = "Star"
	KindPlanet           Kind = "Planet"
	KindMoon             Kind = "Moon"
	KindGalaxy           Kind = "Galaxy"
	KindNebula           Kind = "Nebula"
	KindStarCluster      Kind = "StarCluster"
	KindOpenCluster      Kind = "OpenCluster"
	KindGlobularCluster  Kind = "GlobularCluster"
	KindSupernovaRemnant Kind = "SupernovaRemnant"
	KindAsterism         Kind = "Asterism"
	KindDoubleStar       Kind = "DoubleStar"
	KindOther            Kind = "Other"
)

// Target represents an astronomical object in a resolve.
type Target struct {
	ID             string
	Name           string
	Designation    string
	SPKID          string
	Kind           Kind
	Coord          coord.ICRS
	HasCoord       bool // true if Coord contains valid coordinates
	Epoch          time.Time
	PmRA           angle.Angle // Proper motion in RA
	PmDec          angle.Angle // Proper motion in Declination
	Parallax       angle.Angle
	RadialVelocity float64 // km/s
	Catalog        string
	Aliases        []string

	// TLE data for satellite targets (populated by NORAD provider).
	TLELine1 string
	TLELine2 string

	// Physical magnitude parameters (populated by SBDB or catalog providers).
	// For asteroids: H (absolute mag) and G (slope parameter, default 0.15).
	// For comets: M1/k1 (total magnitude), M2/k2 (nuclear magnitude).
	// For stars: VMag (catalog V-band magnitude).
	VMag    float64 // Catalog V-band magnitude (0 = not set)
	HasVMag bool    // true if VMag is populated
	H       float64 // Asteroid absolute magnitude
	G       float64 // Asteroid slope parameter (0 = not set, use 0.15 default)
	HasH    bool    // true if H is populated
	M1      float64 // Comet total absolute magnitude
	K1      float64 // Comet total activity parameter
	M2      float64 // Comet nuclear absolute magnitude
	K2      float64 // Comet nuclear activity parameter
	HasM1   bool    // true if M1/K1 are populated

	// HG1G2 / sHG1G2 phase-curve parameters (Carry et al. 2024).
	// Populated by: FINK SSOFT provider.
	G1            float64 // Phase function G₁ parameter
	G2            float64 // Phase function G₂ parameter
	HasG1G2       bool    // true if G1/G2 are populated
	SpinRA        float64 // Spin axis RA (degrees, J2000)
	SpinDec       float64 // Spin axis Dec (degrees, J2000)
	HasSpin       bool    // true if spin axis is populated
	Oblateness    float64 // Polar oblateness R ∈ (0,1], 1 = sphere
	HasOblateness bool    // true if R is populated
}

// ICRS implements coord.Object for a static catalog Target.
func (t Target) ICRS(_ time.Time) (coord.ICRS, error) {
	return t.Coord, nil
}

// Provider defines the interface for astronomical catalogs.
type Provider interface {
	Name() string
	Resolve(query string) (Target, bool)
	Search(query string) []Target
}

var (
	ErrNotFound  = errors.New("target not found")
	ErrAmbiguous = errors.New("ambiguous target name")
)

// Normalize converts a query to a canonical form for matching.
func Normalize(query string) string {
	q := strings.ToLower(strings.TrimSpace(query))
	q = strings.ReplaceAll(q, " ", "")
	if strings.HasPrefix(q, "messier") {
		q = "m" + q[7:]
	}
	return q
}

// Score evaluates how well a candidate string matches a target query (0.0 to 1.0).
func Score(query, candidate string) float64 {
	if query == "" || candidate == "" {
		return 0
	}
	c := Normalize(candidate)
	if query == c {
		return 1.0
	}
	if strings.HasPrefix(c, query) {
		return 0.8
	}
	if strings.Contains(c, query) {
		return 0.5
	}
	dist := levenshtein(query, c)
	maxLen := len(query)
	if len(c) > maxLen {
		maxLen = len(c)
	}
	lScore := 1.0 - float64(dist)/float64(maxLen)
	if lScore < 0 {
		lScore = 0
	}
	return lScore * 0.3
}

func levenshtein(s, t string) int {
	d := make([][]int, len(s)+1)
	for i := range d {
		d[i] = make([]int, len(t)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for j := 1; j <= len(t); j++ {
		for i := 1; i <= len(s); i++ {
			cost := 1
			if s[i-1] == t[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, min(d[i][j-1]+1, d[i-1][j-1]+cost))
		}
	}
	return d[len(s)][len(t)]
}
