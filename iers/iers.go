package iers

import (
	"bytes"
	"embed"
	"log"
)

//go:generate go run ../internal/tools/cmd/download/main.go https://datacenter.iers.org/data/9/finals2000A.all data/finals2000A.all

//go:embed all:data/*
var eopFS embed.FS

// FinalsData holds the IERS EOP reference data embedded at build time
// (empty if `go generate ./iers/...` hasn't been run — see README "Data
// downloads & offline usage").
//
//nolint:gochecknoglobals // embedded IERS EOP reference data, populated lazily by loadEmbedded
var FinalsData []byte

// loadEmbedded parses the embedded finals2000A.all snapshot and, if no
// model has been explicitly registered yet, installs it as the default.
// Called lazily and exactly once — the first time GetModel is queried, via
// eop.go's loadOnce — rather than from init(), so merely importing iers
// (transitively, via coord) doesn't pay the ~3.7 MB parse cost when EOP
// data is never actually queried, and so a program that calls
// RegisterModel/LoadFile before its first query never has that choice
// silently overridden.
func loadEmbedded() {
	d, err := eopFS.ReadFile("data/finals2000A.all")
	if err == nil {
		FinalsData = d
	}

	if len(FinalsData) == 0 {
		log.Printf("astrogo/iers: no embedded EOP data — run `go generate ./iers/...`, call iers.FetchNow/LoadFile, or accept ZeroModel's reduced accuracy; see README \"Data downloads & offline usage\"")
		return
	}

	model, err := ParseFinals2000A(bytes.NewReader(FinalsData))
	if err != nil {
		log.Printf("astrogo/iers: failed to parse embedded EOP data: %v", err)
		return
	}

	registerIfDefault(model)
}
