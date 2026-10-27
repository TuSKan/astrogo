#!/usr/bin/env bash
#
# The benchmark shards, and the one place they are defined.
#
# # Why shards
#
# go test runs benchmarks one package at a time, deliberately: cmd/go forces
# benchmark runs serial ("Force benchmarks to run in serial", in
# cmd/go/internal/test), so that two packages never share a CPU and skew each
# other's timings. On one runner that made the job 16.7 minutes (run
# 40435420207, 2026-10-27): 138 benchmarks, six samples each, about 1.2 s a
# sample. No single benchmark was the cause; the largest took 12 s of the 968.
#
# Separate runners are separate machines, so splitting the packages across them
# keeps every benchmark alone on its CPU, with the same count and benchtime as
# before, and brings the wall time down to the largest shard's.
#
# # How they are balanced
#
# By that run's seconds per package: coord 191, plan 189, skybrightness 161,
# time 86, angle 54, vector 43, ephemeris/kepler 14, and 250 for everything
# else.
#
#	coord-angle   coord, angle                    245 s
#	plan          plan, vector, ephemeris/kepler   246 s
#	sky-time      skybrightness, time              247 s
#	rest          everything else                  250 s
#
# rest is every package the named shards leave out, read from go list, so a new
# package is benchmarked without anyone remembering to list it. The balance
# drifts as benchmarks change. That costs wall time, never coverage.
#
# Usage:
#   bench-shards.sh json              the shard names, as a JSON array for a matrix
#   bench-shards.sh packages <shard>  the shard's import paths, one per line

set -euo pipefail

readonly module="github.com/TuSKan/astrogo"

# The named shards, in matrix order. rest is appended after them.
readonly named_shards=(coord-angle plan sky-time)

# shard_members prints a named shard's packages, relative to the module.
shard_members() {
	case "$1" in
	coord-angle) echo coord angle ;;
	plan) echo plan vector ephemeris/kepler ;;
	sky-time) echo skybrightness time ;;
	*) return 1 ;;
	esac
}

# named_packages prints every package a named shard claims, as an import path.
named_packages() {
	local shard pkg

	for shard in "${named_shards[@]}"; do
		for pkg in $(shard_members "$shard"); do
			echo "$module/$pkg"
		done
	done
}

cd "$(git rev-parse --show-toplevel)"

case "${1:-}" in
json)
	# A package in two shards would be benchmarked twice; say so here, where
	# the table is, rather than in a doubled row of the delta.
	dup=$(named_packages | sort | uniq -d)
	if [ -n "$dup" ]; then
		echo "bench-shards.sh: in more than one shard: $dup" >&2
		exit 1
	fi

	printf '['
	for shard in "${named_shards[@]}"; do
		printf '"%s",' "$shard"
	done
	printf '"rest"]\n'
	;;
packages)
	shard="${2:?usage: bench-shards.sh packages <shard>}"

	if [ "$shard" = rest ]; then
		go list ./... | grep -vxF -f <(named_packages)
	elif members=$(shard_members "$shard"); then
		for pkg in $members; do
			echo "$module/$pkg"
		done
	else
		echo "bench-shards.sh: no shard named $shard" >&2
		exit 2
	fi
	;;
*)
	echo "usage: bench-shards.sh json | packages <shard>" >&2
	exit 2
	;;
esac
