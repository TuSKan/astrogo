// Package workload holds astrogo's ephemeris workload benchmarks, the
// measurements behind docs/ephemeris.md. It has no API: everything is in its
// _test.go files, which need the cached JPL kernels and so build only under
// the integration tag.
//
// Each benchmark is a workload a caller actually runs — one body at one time,
// the whole sky at one time, one body over many times, a light-time place,
// a moon about its planet, many targets for one site, an event search, a
// satellite pass, concurrent readers — run against the kernel on disk as the
// library reads it and against the same kernel held in memory, so that the
// cost of reading is separated from the cost of evaluating. Where repeated
// work is the question, a benchmark also reports how many provider calls and
// kernel reads one operation makes.
//
// The experiments that settled docs/ephemeris.md's design questions, which
// logged answers rather than measuring workloads, were removed once every
// decision they informed had shipped (#891). Commit 3b1d36ca has them.
package workload
