#!/usr/bin/env bash
set -euo pipefail

THRESHOLD="${COVERAGE_THRESHOLD:-80}"
PACKAGES=$(go list ./... | grep -v /cmd/server | grep -v /internal/testutil)

go test ${PACKAGES} -coverprofile=coverage.out -covermode=atomic "$@"

TOTAL=$(go tool cover -func=coverage.out | awk '/^total:/ {print substr($3, 1, length($3)-1)}')
echo "Total coverage: ${TOTAL}% (threshold: ${THRESHOLD}%)"

awk -v total="$TOTAL" -v threshold="$THRESHOLD" 'BEGIN {
  if (total + 0 < threshold + 0) {
    printf "Coverage %.1f%% is below required %d%%\n", total, threshold
    exit 1
  }
}'
