package resolve

import (
	"context"
	"errors"
	"strings"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// Kind represents the type of an astronomical object.
type Kind string

const (
	// KindStar represents a star.
	KindStar Kind = "Star"
	// KindPlanet represents a planet.
	KindPlanet Kind = "Planet"
	// KindMoon represents a moon.
	KindMoon Kind = "Moon"
	// KindGalaxy represents a galaxy.
	KindGalaxy Kind = "Galaxy"
	// KindNebula represents a nebula.
	KindNebula Kind = "Nebula"
	// KindStarCluster represents a star cluster.
	KindStarCluster Kind = "StarCluster"
	// KindOpenCluster represents an open cluster.
	KindOpenCluster Kind = "OpenCluster"
	// KindGlobularCluster represents a globular cluster.
	KindGlobularCluster Kind = "GlobularCluster"
	// KindSupernovaRemnant represents a supernova remnant.
	KindSupernovaRemnant Kind = "SupernovaRemnant"
	// KindAsterism represents an asterism.
	KindAsterism Kind = "Asterism"
	// KindDoubleStar represents a double star.
	KindDoubleStar Kind = "DoubleStar"
	// KindAsteroid represents a minor planet (asteroid).
	KindAsteroid Kind = "Asteroid"
	// KindComet represents a comet.
	KindComet Kind = "Comet"
	// KindSatellite represents an artificial Earth satellite.
	KindSatellite Kind = "Satellite"
	// KindOther represents other celestial objects.
	KindOther Kind = "Other"
)

// Target represents an astronomical object in a resolve.
type Target struct {
	// Epoch is the epoch of the target.
	Epoch time.Time
	// Catalog is the catalog of the target.
	Catalog string
	// Name is the name of the target.
	Name string
	// Designation is the designation of the target.
	Designation string
	// SPKID is the SPKID of the target.
	SPKID string
	// Kind is the kind of the target.
	Kind Kind
	// ID is the ID of the target.
	ID string
	// TLELine2 is the TLE line 2 of the target.
	TLELine2 string
	// TLELine1 is the TLE line 1 of the target.
	TLELine1 string
	// Aliases are the aliases of the target.
	Aliases []string
	// Coord is the coordinate of the target.
	Coord coord.ICRS
	// VMag is the V magnitude of the target.
	VMag float64
	// G2 is the G2 of the target.
	G2 float64
	// Parallax is the parallax of the target.
	Parallax angle.Angle
	// PmDec is the proper motion in declination of the target.
	PmDec angle.Angle
	// PmRA is the proper motion in right ascension of the target.
	PmRA angle.Angle
	// Oblateness is the oblateness of the target.
	Oblateness float64
	// SpinDec is the spin declination of the target.
	SpinDec float64
	// H is the absolute magnitude of the target.
	H float64
	// G is the phase coefficient of the target.
	G float64
	// SpinRA is the spin right ascension of the target.
	SpinRA float64
	// M1 is the total magnitude of the target.
	M1 float64
	// K1 is the phase coefficient of the target.
	K1 float64
	// M2 is the nuclear magnitude of the target.
	M2 float64
	// K2 is the phase coefficient of the target.
	K2 float64
	// RadialVelocity is the radial velocity of the target.
	RadialVelocity float64
	// G1 is the phase coefficient of the target.
	G1 float64
	// HasM1 is true if the target has M1.
	HasM1 bool
	// HasG1G2 is true if the target has G1 and G2.
	HasG1G2 bool
	// HasH is true if the target has H.
	HasH bool
	// HasVMag is true if the target has VMag.
	HasVMag bool
	// HasSpin is true if the target has spin information.
	HasSpin bool
	// HasCoord is true if the target has coordinate information.
	HasCoord bool
	// HasOblateness is true if the target has oblateness information.
	HasOblateness bool
	// Provenance maps each populated field name to the provider name
	// (Provider.Name(), never Target.Catalog) that contributed its value
	// in a merged Target. Nil for a Target sourced from a single provider.
	Provenance map[string]string
}

// ICRS implements coord.Object for a static catalog Target.
func (t Target) ICRS(_ time.Time) (coord.ICRS, error) {
	return t.Coord, nil
}

// Provider defines the interface for astronomical catalogs.
type Provider interface {
	Name() string
	Resolve(ctx context.Context, query string) (Target, bool)
	Search(ctx context.Context, query string) []Target
}

var (
	// ErrNotFound is returned when no provider can resolve the query.
	ErrNotFound = errors.New("target not found")
	// ErrAmbiguous is returned when a query matches multiple targets.
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
