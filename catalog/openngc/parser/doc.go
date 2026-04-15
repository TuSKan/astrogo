// Package main provides the CSV generation tool for the OpenNGC dataset.
//
// It downloads the raw OpenNGC semicolon-delimited data from the upstream
// repository, parses object types and coordinates, and writes a compact
// runtime CSV used by the openngc package via go:embed.
//
// Usage:
//
//	go run parse.go <output_path> <url1> [url2...]
package main
