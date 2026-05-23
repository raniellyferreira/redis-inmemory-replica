#!/usr/bin/env bash
#
# check-fieldalignment.sh — run golang.org/x/tools fieldalignment on the
# project's production .go packages.
#
# Per ADR 0001 D-10 §10.5: "struct field alignment vet" is a P0 pre-flight
# gate. Misaligned struct layouts waste memory on the hot path (Value,
# request/response types, replication state). This script surfaces
# violations; it does NOT auto-fix.
#
# Modes:
#   --report-only (default): always exit 0; print findings.
#                            Use during P0 of the v2 refactor.
#   --blocking:              exit non-zero on ANY production misalignment.
#                            Reserved for later phases.
#
# Fixes land per-PR, per-subsystem, with `benchstat` before/after, as
# part of P3/P3a/P3b of the v2 refactor.

set -euo pipefail

MODE="report-only"

while [ $# -gt 0 ]; do
  case "$1" in
    --report-only) MODE="report-only"; shift ;;
    --blocking)    MODE="blocking"; shift ;;
    -h|--help) sed -n '3,19p' "$0"; exit 0 ;;
    *) echo "unknown flag: $1" >&2; exit 2 ;;
  esac
done

FA_BIN="${FA_BIN:-${HOME}/go/bin/fieldalignment}"
if ! [ -x "$FA_BIN" ]; then
  echo "Installing fieldalignment..."
  go install golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment@latest
fi

PKGS=(
  "."
  "./storage/..."
  "./protocol/..."
  "./lua/..."
  "./replication/..."
  "./server/..."
)

echo "==================================================================="
echo "  ADR 0001 D-10 §10.5 — Struct Field Alignment (${MODE} mode)"
echo "  Scanning: ${PKGS[*]}"
echo "==================================================================="
echo

findings_file=$(mktemp)
trap 'rm -f "$findings_file"' EXIT

set +e
"$FA_BIN" "${PKGS[@]}" 2> "$findings_file"
fa_exit=$?
set -e

if [ ! -s "$findings_file" ]; then
  echo "OK — fieldalignment found no misalignments."
  exit 0
fi

prod=$(grep -v "_test\.go:" "$findings_file" || true)
tests=$(grep "_test\.go:" "$findings_file" || true)
prod_count=$(printf '%s\n' "$prod" | grep -c . || true)
test_count=$(printf '%s\n' "$tests" | grep -c . || true)

if [ -n "$prod" ]; then
  echo ">>> PRODUCTION CODE — struct alignment can be improved"
  echo "$prod" | sed 's|^|    |'
  echo
  echo "    Total production findings: ${prod_count}"
  echo
fi

if [ -n "$tests" ]; then
  echo ">>> TEST CODE (informational; not a P0 priority)"
  echo "$tests" | sed 's|^|    |'
  echo
  echo "    Total test findings: ${test_count}"
  echo
fi

echo "Fix strategy (per ADR P3/P3a/P3b):"
echo "  - One PR per subsystem (storage -> protocol -> lua -> replication -> server)"
echo "  - Run benchstat before/after on the touched package"
echo "  - Verify zero allocation regression on hot paths (ADR D-10 §10.1)"
echo

if [ "$MODE" = "blocking" ] && [ "$prod_count" -gt 0 ]; then
  echo "FAIL: ${prod_count} production-code misalignment(s) detected." >&2
  exit 1
fi

echo "Report-only mode — no failure raised (fa exit was ${fa_exit})."
exit 0
