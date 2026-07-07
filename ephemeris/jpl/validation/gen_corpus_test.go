//go:build network

package jpl_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type CorpusEntry struct {
	TargetID    int
	TargetName  string
	EpochStr    string
	ObserverLon float64
	ObserverLat float64
	ObserverEle float64
	GeoVector   [3]float64
	GeoVelocity [3]float64
	Data        *ObserverPoint
}

func TestGenerateCorpus(t *testing.T) {
	requireHorizons(t)

	cases := []CorpusEntry{
		{
			TargetID:    499,
			TargetName:  "Mars",
			EpochStr:    "2024-11-01 12:00",
			ObserverLat: 51.477,
			ObserverLon: 0.0,
			ObserverEle: 0.0,
		},
		{
			TargetID:    301, // Moon
			TargetName:  "Moon",
			EpochStr:    "2025-01-01 00:00",
			ObserverLat: 45.0,
			ObserverLon: 0.0,
			ObserverEle: 0.0,
		},
		{
			TargetID:    599,
			TargetName:  "Jupiter",
			EpochStr:    "2022-03-20 15:00",
			ObserverLat: 0.0,
			ObserverLon: -78.0,
			ObserverEle: 0.0,
		},
	}

	for i := range cases {
		c := &cases[i]
		stopTime := c.EpochStr[:14] + "01"

		t.Logf("Downloading Baseline %d for %s...", i, c.TargetName)
		data, err := fetchObserverTable(c.TargetID, c.TargetName, c.ObserverLon, c.ObserverLat, c.ObserverEle, c.EpochStr, stopTime)
		if err != nil {
			t.Fatalf("Horizons rejected the query: %v", err)
		}
		c.Data = data

		vecData, err := fetchVector(c.TargetID, c.TargetName, c.EpochStr, stopTime)
		if err != nil {
			t.Fatalf("Horizons rejected Vector query: %v", err)
		}
		copy(c.GeoVector[:], vecData.Pos)
		copy(c.GeoVelocity[:], vecData.Vel)
	}

	bytes, err := json.MarshalIndent(cases, "", "  ")
	if err != nil {
		t.Fatalf("Failed to encode JSON: %v", err)
	}

	err = os.MkdirAll("corpus", 0755)
	if err != nil {
		t.Fatalf("Failed to make directory: %v", err)
	}

	path := filepath.Join("corpus", "horizons_edgecases.json")
	if err = os.WriteFile(path, bytes, 0644); err != nil {
		t.Fatalf("Failed to write corpus file: %v", err)
	}

	t.Logf("Successfully locked %d astronomical baselines to %s", len(cases), path)
}
