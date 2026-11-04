package kepler

import (
	"sync"

	"github.com/TuSKan/astrogo/logging"
)

// sofaSpan is one SOFA theory's fitted span, and what SOFA says of its
// accuracy outside it, for the warning a date outside the span gets.
type sofaSpan struct {
	once     *sync.Once
	theory   string
	span     string
	accuracy string
}

var (
	// Epv00's status is +1 outside 1900-2100 AD. Its notes give the
	// deterioration against DE406.
	epv00Span = sofaSpan{
		once:   new(sync.Once),
		theory: "Epv00",
		span:   "1900-2100 AD",
		accuracy: "SOFA: position errors about double by 1800 and 2200, " +
			"10 times by 1500 and 2500, and 60 times by 1000 and 3000",
	}

	// Plan94's status is +1 outside 1000-3000 AD.
	plan94Span = sofaSpan{
		once:   new(sync.Once),
		theory: "Plan94",
		span:   "1000-3000 AD",
		accuracy: "SOFA: no worse than 1.5 times its 1800-2050 accuracy inside the span, " +
			"and declining outside it",
	}
)

// warnOutsideSpan writes, once per process and per theory, that a SOFA theory
// was evaluated at a date outside its fitted span.
//
// SOFA calls this a warning and still returns a position, which is right for
// a planning-grade provider: 1800 and 2200 are ordinary dates to plan for, and
// refusing them would be wrong. But the position is worse than inside the
// span, by a factor SOFA states, and Locate's signature has no room to say so.
// Until #1003 nothing did. A warning through logging is how astrogo already
// reports an accuracy that silently degraded (time's EOP-unavailable warning),
// and once is enough: a scan over a century would otherwise write one per
// sample.
func warnOutsideSpan(s sofaSpan, jd float64) {
	s.once.Do(func() {
		logging.Warn("SOFA theory evaluated outside its fitted span",
			"theory", s.theory,
			"jd", jd,
			"span", s.span,
			"accuracy", s.accuracy,
			"remedy", "an SPK kernel for dates outside the span: ephemeris.NewProvider with ephemeris.Planets")
	})
}
