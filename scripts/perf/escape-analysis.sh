#!/usr/bin/env bash
#
# escape-analysis.sh — produce a CURRENT escape-analysis snapshot and
# diff it against the most recent baseline (artifacts/baseline/latest/
# escape-analysis.txt). Per ADR 0001 D-10 §10.4.
#
# Designed for fast iteration during a PR: run before and after a change
# in a hot package to see which allocations moved from stack to heap
# (or, ideally, the other way).
#
# Usage:
#   ./scripts/perf/escape-analysis.sh                    # diff vs latest baseline
#   ./scripts/perf/escape-analysis.sh path/to/old.txt    # diff vs a specific file
#
# Exit code is 0 even if there are differences — this is informational.
# `bench-regression.yml` (later PR) will gate on benchstat instead.

set -euo pipefail

BASELINE="${1:-artifacts/baseline/latest/escape-analysis.txt}"
CURRENT_DIR="/tmp/escape-analysis-current"
mkdir -p "${CURRENT_DIR}"
CURRENT_FILE="${CURRENT_DIR}/escape-analysis.txt"

if [ ! -f "${BASELINE}" ]; then
  echo "Baseline file not found: ${BASELINE}" >&2
  echo "Run \`make baseline\` first to capture one." >&2
  exit 2
fi

echo "Capturing current escape-analysis snapshot..."
go build -gcflags="-m=2" ./storage/... ./protocol/... ./lua/... ./replication/... ./server/... ./ \
  > "${CURRENT_FILE}" 2>&1 || true
echo "  → ${CURRENT_FILE} ($(wc -l < "${CURRENT_FILE}") lines)"
echo

echo "==================================================================="
echo "  Escape-analysis diff (vs ${BASELINE})"
echo "==================================================================="

# Show summary stats first (count of "escapes to heap" lines)
base_heap=$(grep -c 'escapes to heap' "${BASELINE}" || true)
curr_heap=$(grep -c 'escapes to heap' "${CURRENT_FILE}" || true)
delta=$(( curr_heap - base_heap ))

echo "  'escapes to heap' lines:"
echo "    baseline: ${base_heap}"
echo "    current : ${curr_heap}"
if [ "${delta}" -gt 0 ]; then
  echo "    delta   : +${delta}  ⚠ MORE escapes than baseline"
elif [ "${delta}" -lt 0 ]; then
  echo "    delta   : ${delta}   ✓ FEWER escapes than baseline"
else
  echo "    delta   : 0          (no change)"
fi
echo

# Unified diff for the human reviewer
if diff -u "${BASELINE}" "${CURRENT_FILE}" > /tmp/escape-diff.patch 2>&1; then
  echo "OK — no differences in escape analysis output."
else
  echo "Differences detected. Unified diff saved to /tmp/escape-diff.patch"
  echo
  echo "First 80 lines of the diff:"
  head -80 /tmp/escape-diff.patch
  echo
  echo "(See /tmp/escape-diff.patch for the full diff.)"
fi
