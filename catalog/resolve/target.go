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
	Epoch          time.Time
	Catalog        string
	Name           string
	Designation    string
	SPKID          string
	Kind           Kind
	ID             string
	TLELine2       string
	TLELine1       string
	Aliases        []string
	Coord          coord.ICRS
	VMag           float64
	G2             float64
	Parallax       angle.Angle
	PmDec          angle.Angle
	PmRA           angle.Angle
	Oblateness     float64
	SpinDec        float64
	H              float64
	G              float64
	SpinRA         float64
	M1             float64
	K1             float64
	M2             float64
	K2             float64
	RadialVelocity float64
	G1             float64
	HasM1          bool
	HasG1G2        bool
	HasH           bool
	HasVMag        bool
	HasSpin        bool
	HasCoord       bool
	HasOblateness  bool
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

	maxLen := max(len(c), len(query))

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
