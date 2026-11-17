// Example: Evaluate lunar crescent visibility from computed ephemeris.
//
// This finds the next New Moon, then evaluates crescent visibility on the
// following evening from Quinta Calixto with CrescentVisibility, which finds
// the sunset and moonset and evaluates 18 published criteria (1910–2021),
// each in the convention it was defined in.
//
// Reference:
//
//	Al-Jumaili et al., "A Review on Modern Lunar Crescent Visibility
//	Criterion", Malaysian Journal of Science, Vol. 41(3), 2022.
package main

import (
	"fmt"
	"log"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/examples/internal/show"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

func main() {
	// This example runs offline, so it uses no Earth-orientation data
	// (UT1 = UTC, no polar motion). That can move the Moon's and Sun's
	// altitudes by up to 13.5 arcsec, far below the tenths of a degree the
	// criteria work in. For full accuracy, blank-import remote/eop and
	// consent to the IERS bulletin, as 09_geometry_events does.
	time.RegisterModel(time.ZeroModel{})

	// ── Observer: Quinta Calixto, Brazil ─────────────────────────────────
	site, err := plan.NewSiteEarthLocation("Quinta Calixto", -22.528478, -46.473002, 835.05)
	if err != nil {
		log.Fatalf("site: %v", err)
	}

	prov := eph.Default()

	// ── Find the first New Moon of December 2026 ────────────────────────
	// A fixed start keeps the output the same on every run; start from
	// time.NowUTC() to look for the next one from today. Times print in
	// the zone of the start, here the site's.
	tz, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		log.Fatalf("tz: %v", err)
	}

	from := time.Date(2026, 12, 1, 0, 0, 0, 0, tz)

	newMoon, err := plan.NextNewMoon(from, prov)
	if err != nil {
		log.Fatalf("NextNewMoon: %v", err)
	}

	// ── Evaluate the first evening after the New Moon ───────────────────
	// The crescent is typically first visible on the evening following
	// the astronomical New Moon (conjunction). CrescentVisibility takes the
	// first sunset after the instant it is given, the moonset after it, and
	// Yallop's best time between them, 4/9 of the lag after sunset, when
	// the sky is dark enough to see a thin crescent but the Moon is still
	// above the horizon.
	result, err := plan.CrescentVisibility(newMoon.Time, site, prov)
	if err != nil {
		log.Fatalf("CrescentVisibility: %v", err)
	}

	const clock = "2006-01-02 15:04 MST"

	show.Title("Lunar crescent visibility", site.Name()+", the evening after December 2026's New Moon")

	show.Section("The evening")
	show.Fields(
		show.KV("New Moon", "%s", newMoon.Time.Format(clock)),
		show.KV("Sunset", "%s", result.Sunset.Format(clock)),
		show.KV("Moonset", "%s", result.Moonset.Format(clock)),
		show.KV("Best time", "%s", result.BestTime.Format(clock)),
	)

	// ── Yes-or-no criteria ──────────────────────────────────────────────
	// Each reads the geometry in its own convention: at sunset, at the best
	// time, or with the Moon's altitude measured to its lower limb. That is
	// why the arc of vision differs between rows of the same evening.
	verdicts := []struct {
		name string
		v    plan.CrescentVerdict
	}{
		{"Fotheringham (1910)", result.Fotheringham},
		{"Maunder (1911)", result.Maunder},
		{"Danjon (1932)", result.Danjon},
		{"Bruin (1977)", result.Bruin},
		{"Ilyas (1983)", result.Ilyas1983},
		{"Ilyas (1988)", result.Ilyas1988},
		{"MABIMS (1995)", result.MABIMS1995},
		{"Fatoohi (1998)", result.Fatoohi1998},
		{"SAAO naked (2001)", result.CaldwellNakedEye},
		{"SAAO aided (2001)", result.CaldwellOptical},
		{"Krauss Athens (2012)", result.KraussAthenian},
		{"Istanbul (2016)", result.Istanbul2016},
		{"Alrefay naked (2018)", result.AlrefayNakedEye},
		{"Alrefay aided (2018)", result.AlrefayOpticalAid},
		{"MABIMS (2021)", result.MABIMS2021},
	}

	show.Section("Criteria")

	tab := show.NewTable(show.Left("Criterion"), show.Left("Verdict"),
		show.Right("ARCV"), show.Right("ARCL"), show.Right("DAZ"), show.Right("Moon"), show.Right("Width"))

	for _, c := range verdicts {
		verdict := "not visible"
		if c.v.Visible {
			verdict = "visible"
		}

		p := c.v.Params
		tab.Row(c.name, verdict,
			fmt.Sprintf("%.2f°", p.ArcV.Degrees()), fmt.Sprintf("%.2f°", p.ArcL.Degrees()),
			fmt.Sprintf("%.2f°", p.DAZ.Degrees()), fmt.Sprintf("%.2f°", p.MAlt.Degrees()),
			fmt.Sprintf("%.2f′", p.W.Arcminutes()))
	}

	tab.Print()

	// ── Zone criteria ───────────────────────────────────────────────────
	// Three criteria grade the evening rather than answer yes or no, each
	// from one discriminant: Yallop's q, Odeh's V and Qureshi's S.
	show.Section("Zones")

	zones := show.NewTable(show.Left("Criterion"), show.Left("Zone"), show.Right("Value"), show.Left("Meaning"))
	zones.Row("Yallop (1997), q", result.Yallop.Code, fmt.Sprintf("%.4f", result.Yallop.Value), result.Yallop.Label)
	zones.Row("Odeh (2004), V", result.Odeh.Code, fmt.Sprintf("%.4f", result.Odeh.Value), result.Odeh.Label)
	zones.Row("Qureshi (2010), S", result.Qureshi.Code, fmt.Sprintf("%.4f", result.Qureshi.Value), result.Qureshi.Label)
	zones.Print()
	fmt.Println()

	show.Note("ARCV is the arc of vision, the Moon's altitude above the Sun's;",
		"ARCL the arc of light, their separation; DAZ their difference in",
		"azimuth; Moon the Moon's altitude; Width the crescent's width.")
}
