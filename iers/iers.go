package iers

import (
	"bytes"
	"embed"
	"log"
)

//go:generate go run ../internal/tools/download.go https://datacenter.iers.org/data/9/finals2000A.all data/finals2000A.all

//go:embed data/*
var eopFS embed.FS

var FinalsData []byte

func init() {
	d, err := eopFS.ReadFile("data/finals2000A.all")
	if err == nil {
		FinalsData = d
	}

	if len(FinalsData) == 0 {
		return // in case go generate hasn't been run or file was empty
	}
	model, err := ParseFinals2000A(bytes.NewReader(FinalsData))
	if err != nil {
		log.Printf("astrogo/earth/iers: failed to parse embedded EOP data: %v", err)
		return
	}
	RegisterModel(model)
}
