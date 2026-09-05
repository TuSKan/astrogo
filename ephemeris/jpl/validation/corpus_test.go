//go:build validation || network

package jpl_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/TuSKan/astrogo/constants"
)

// This file is built under either tag and neither alone.
//
// The generator is network-tagged and the consumer is validation-tagged, so
// the two cannot see each other's files. A constraint of "validation ||
// network" is what lets one declaration serve both: an untagged file would
// serve them too, but then an ordinary "golangci-lint run" — which is the
// gate CI actually runs, with no tags — sees a file whose every symbol has
// no consumer, and reports the lot as unused. They previously coped by each
// declaring its own copy of the on-disk shape — CorpusEntry on one side,
// RegressionEntry plus BaselinePoint on the other, with a comment noting the
// duplication was there "to bypass network blocks". Two independent
// declarations of one serialisation format drift silently: a field renamed on
// the writing side decodes as a zero on the reading side, and a corpus of
// zeroes still parses.

// kmPerAU converts the AU-valued state differences these suites measure into
// the kilometres that the reference routines' own accuracy figures are quoted
// in. The idiom is the one the constants package's doc demonstrates; Value is
// in metres.
//
// Here rather than in main_test.go because the validation- and network-tagged
// suites both need it and cannot see each other's files, and main_test.go has
// to stay untagged so TestMain always compiles.
var kmPerAU = constants.IAU.AstronomicalUnit.Value / 1e3

// corpusSchemaVersion is the version of the document below.
//
// Checked on read and never guessed at. A reader that silently reinterprets
// an older layout produces a comparison between two things that were not
// measured the same way, which is worse than refusing to compare at all.
const corpusSchemaVersion = 1

// corpusPath is where the document lives, relative to this package.
var corpusPath = filepath.Join("corpus", "horizons.json")

// Errors from reading the corpus.
var (
	errCorpusSchema  = errors.New("corpus: unsupported schema version")
	errCorpusMissing = errors.New("corpus: not found")
)

// samplingClass records why an epoch is in the corpus.
//
// Kept per entry rather than only in the manifest so a failure says which
// kind of case broke. A regression at a regularly sampled epoch and one at a
// leap-second boundary point at very different causes, and a flat list of
// epochs cannot tell them apart.
type samplingClass string

const (
	// classRegular is even sampling across a span, which catches secular
	// drift.
	classRegular samplingClass = "regular"

	// classBoundary is an epoch chosen because something changes there: a
	// leap second, J2000, the edge of measured Earth-orientation data.
	// Mandatory rather than nice to have — algorithms fail at their
	// boundaries far more often than in the middle, and even sampling
	// almost never lands on one.
	classBoundary samplingClass = "boundary"

	// classAdversarial is a geometry chosen to be hard: near the horizon,
	// near the zenith, circumpolar, across the right-ascension wrap.
	classAdversarial samplingClass = "adversarial"
)

// corpusSite is an observing site with a name and a reason for being one.
//
// Named rather than being a bare triple of numbers: an anonymous coordinate
// in a corpus cannot be reviewed, and a failure that says "at 19.83, -155.47"
// is a lookup away from saying "at Mauna Kea".
type corpusSite struct {
	Name       string  `json:"name"`
	Lon        float64 `json:"lon_deg"`
	Lat        float64 `json:"lat_deg"`
	Height     float64 `json:"height_m"`
	Provenance string  `json:"provenance"`
}

// corpusPoint is Horizons' answer at one epoch, from one site, for one body.
type corpusPoint struct {
	AstroRA   float64 `json:"astro_ra_deg"`
	AstroDec  float64 `json:"astro_dec_deg"`
	AppRA     float64 `json:"app_ra_deg"`
	AppDec    float64 `json:"app_dec_deg"`
	Azimuth   float64 `json:"azimuth_deg"`
	Elevation float64 `json:"elevation_deg"`
	Range     float64 `json:"range_au"`
}

// corpusEntry is one comparison case.
type corpusEntry struct {
	Class      samplingClass `json:"class"`
	TargetID   int           `json:"target_id"`
	TargetName string        `json:"target_name"`

	// SiteName references an entry in the manifest's Sites table rather
	// than embedding it. Every site carries a name and a paragraph of
	// provenance, and repeating those across a few hundred entries tripled
	// the document for no information: the first version of this file was
	// 235 KB, of which the great majority was the same five provenance
	// strings over and over.
	SiteName string `json:"site"`

	// EpochJDUT is the instant, as the Julian Date in Universal Time that
	// Horizons prints in column 0 of both table types.
	//
	// The scale is in the name because getting it wrong is invisible and
	// expensive. Both queries send TIME_TYPE='UT', under which Horizons
	// labels the column JDUT — "Julian Day Number, Universal Time" — rather
	// than the JDTDB it prints by default for vector tables. Reading that
	// value as TDB applies a 69-second error, which at Earth's rotation
	// rate is about 1040 arcseconds of hour angle: a topocentric comparison
	// that should agree to arcseconds instead disagrees by a sixth of a
	// degree, uniformly, with nothing in the number to say why.
	//
	// Stored as the service's own number rather than as a formatted string,
	// so nothing has to be parsed back and no format can silently imply a
	// scale the data does not have.
	EpochJDUT float64 `json:"epoch_jd_ut"`

	// GeoVector and GeoVelocity are the geocentric state Horizons reports
	// for the same instant, in AU and AU/day, ICRF.
	GeoVector   [3]float64 `json:"geo_vector_au"`
	GeoVelocity [3]float64 `json:"geo_velocity_au_per_day"`

	Observed corpusPoint `json:"observed"`
}

// key identifies an entry across regenerations, so two corpora can be
// compared entry by entry rather than as opaque blobs.
func (e corpusEntry) key() string {
	return fmt.Sprintf("%s|%s|%s|JD%.6f", e.Class, e.TargetName, e.SiteName, e.EpochJDUT)
}

// corpusManifest records where the document came from.
//
// # What is pinned here, and what deliberately is not
//
// Pinned: the reference service and the exact query shape, the astrogo commit
// that generated it, the sampling design, and the date. Those are what make
// the document reproducible.
//
// Not pinned, on purpose: any planetary kernel or its hash. This corpus holds
// Horizons' own answers, and no astrogo kernel takes part in producing them —
// the consumer feeds Horizons' geocentric state through a linear mock
// provider precisely so the comparison isolates the apparent-place and
// topocentric stages. Recording a kernel SHA-256 that played no part would be
// provenance theatre, and a reader would reasonably infer the corpus had been
// validated against that kernel.
//
// Also not pinned: Earth-orientation data. The consumer builds a coord.Context
// and so depends on whatever IERS series is on the machine at the time, which
// is a real sensitivity of the comparison and cannot be frozen into the
// document. It is named here so nobody has to rediscover it from a failure.
type corpusManifest struct {
	SchemaVersion int    `json:"schema_version"`
	Generated     string `json:"generated"`
	Commit        string `json:"astrogo_commit"`

	Reference      string `json:"reference"`
	ReferenceQuery string `json:"reference_query"`
	Refraction     string `json:"refraction"`

	// Sites is the observer table every entry references by name.
	Sites []corpusSite `json:"sites"`

	// Sampling describes each span and why it is in the design.
	//
	// There is no seed, and no pseudo-random epoch class, because Horizons
	// serves evenly spaced series: a random epoch costs a whole request, so
	// a few hundred of them would mean a few hundred requests to somebody
	// else's server for no more coverage than boundary and adversarial
	// sampling already give. Recording a seed that selected nothing would
	// be worse than recording none.
	Sampling string `json:"sampling"`

	NotPinned []string `json:"not_pinned"`
}

// corpus is the whole document.
type corpus struct {
	Manifest corpusManifest `json:"manifest"`
	Entries  []corpusEntry  `json:"entries"`
}

// sortEntries puts the document in a stable order.
//
// Deterministic ordering is what makes a regenerated corpus reviewable:
// without it every regeneration is a whole-file diff and nobody can see which
// numbers actually moved.
func (c *corpus) sortEntries() {
	sort.Slice(c.Entries, func(i, j int) bool {
		return c.Entries[i].key() < c.Entries[j].key()
	})
}

// loadCorpus reads and validates the document.
func loadCorpus() (*corpus, error) {
	data, err := os.ReadFile(corpusPath)
	if err != nil {
		return nil, fmt.Errorf("%w at %s: %w", errCorpusMissing, corpusPath, err)
	}

	var c corpus
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("corpus: decoding %s: %w", corpusPath, err)
	}

	if c.Manifest.SchemaVersion != corpusSchemaVersion {
		return nil, fmt.Errorf("%w: document is version %d, this build reads version %d",
			errCorpusSchema, c.Manifest.SchemaVersion, corpusSchemaVersion)
	}

	return &c, nil
}

// site resolves an entry's site against the manifest.
//
// An entry naming a site the manifest does not define is a corrupt document,
// not a missing map lookup to paper over: the coordinates are what the
// comparison is computed from, and defaulting them to zero would silently
// move every observer to the Gulf of Guinea.
func (c *corpus) site(name string) (corpusSite, error) {
	for _, s := range c.Manifest.Sites {
		if s.Name == name {
			return s, nil
		}
	}

	return corpusSite{}, fmt.Errorf("%w: entry names site %q, which the manifest does not define",
		errCorpusSchema, name)
}
