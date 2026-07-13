// Package main implements the download tool used by go:generate directives
// to fetch development-time data snapshots (IERS EOP, OpenNGC CSVs).
// Invoking this tool IS the download consent — it always uses
// remote.DownloadURL directly, bypassing the runtime endpoint registry.
package main

import (
	"context"
	"log"
	"os"

	gofs "github.com/ungerik/go-fs"

	"github.com/TuSKan/astrogo/remote"
)

func main() {
	if len(os.Args) < 3 {
		log.Fatal("Usage: go run main.go <url> <path>")
	}

	err := remote.DownloadURL(context.Background(), os.Args[1], gofs.File(os.Args[2]))
	if err != nil {
		log.Fatal(err)
	}
}
