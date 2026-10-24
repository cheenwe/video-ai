#!/usr/bin/env bash
# Reproducible performance benchmarks for the video-AI hot paths.
#
# The numbers were produced on an Apple M1 Pro (darwin/arm64, 8 cores) with a
# 1280x720 JPEG frame letterboxed into 640x640. Run it before and after a
# change to the inference pipeline so improvements are measured, not assumed.
#
# Usage:
#   ./scripts/video-ai-bench.sh            # default 300 iterations
#   BENCH_TIME=1000x ./scripts/video-ai-bench.sh
#
# Output is appended to bench-results.txt in the repo root (gitignored data
# directories aside, this file is meant to be compared by hand).
set -eo pipefail

cd "$(dirname "$0")/../backend"

BENCH_TIME="${BENCH_TIME:-300x}"
OUT="${OUT:-../bench-results.txt}"

# CGO is off on this project's build (no CGO toolchain dependency), and must be
# off here too so the numbers match CI and local builds.
export CGO_ENABLED=0

echo "== $(date '+%F %T')  go=$(go env GOVERSION)  $(go env GOARCH)  benchtime=$BENCH_TIME" | tee -a "$OUT"

go test ./internal/videoai/... \
  -run '^$' \
  -bench 'BenchmarkPreprocess|BenchmarkPostprocess|BenchmarkNMS|BenchmarkHubPublish|BenchmarkJpegScannerFeed' \
  -benchtime="$BENCH_TIME" \
  -benchmem \
  -count=1 2>&1 | grep -E "^(Benchmark|ok|PASS|FAIL|pkg|cpu)" | tee -a "$OUT"

echo "== done" | tee -a "$OUT"
