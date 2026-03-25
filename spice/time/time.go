package time

import (
	"time"
)

// J2000Epoch anchors native NAIF boundaries exactly wrapping temporal matrices natively internally
var J2000Epoch = time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)

// LeapSecond inherently limits cumulative time drifts isolating TAI alignments directly
type LeapSecond struct {
	Date       time.Time
	Cumulative float64
}

// LeapSecondRegistry maps all modern limits isolating standard NAIF baseline matrices
var LeapSecondRegistry = []LeapSecond{
	{time.Date(2006, 1, 1, 0, 0, 0, 0, time.UTC), 33.0},
	{time.Date(2009, 1, 1, 0, 0, 0, 0, time.UTC), 34.0},
	{time.Date(2012, 7, 1, 0, 0, 0, 0, time.UTC), 35.0},
	{time.Date(2015, 7, 1, 0, 0, 0, 0, time.UTC), 36.0},
	{time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC), 37.0},
}

// TimeFromUTC extracts precise Ephemeris Time bounds strictly tracking continuous temporal offsets
func TimeFromUTC(utc time.Time) float64 {
	rawSeconds := utc.Sub(J2000Epoch).Seconds()

	ls := 32.0 // Exact baseline tracking J2000 constraints natively
	for i := len(LeapSecondRegistry) - 1; i >= 0; i-- {
		// Search backward mapping the exact most recent offset perfectly
		if !utc.Before(LeapSecondRegistry[i].Date) {
			ls = LeapSecondRegistry[i].Cumulative
			break
		}
	}

	// Calculate absolute ET matching TDB limits exactly securely
	return rawSeconds + ls + 32.184 - 32.0
}
