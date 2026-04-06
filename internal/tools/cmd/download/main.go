package main

import (
	"log"
	"os"

	"github.com/TuSKan/astrogo/internal/tools"
)

func main() {
	if len(os.Args) < 3 {
		log.Fatal("Usage: go run main.go <url> <path>")
	}
	err := tools.Download(os.Args[1], os.Args[2])
	if err != nil {
		log.Fatal(err)
	}
}
