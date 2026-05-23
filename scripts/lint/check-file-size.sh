#!/usr/bin/env bash
#
# check-file-size.sh — enforce ADR 0001 D-11 file-size budget on .go files.
#
# Soft target: 350 lines (warn; informational only)
# Hard cap:    700 lines (fail in blocking mode; warn in report-only mode)
#
# `_test.go` files are exempt — table-driven tests and fixture data
# legitimately grow them. Production code does not get the same indulgence.
#
# Modes:
#   --report-only (default): always exit 0; print findings to stdout.
#                            Use this in P0 of the v2 refactor.
#   --blocking:              exit non-zero when any file exceeds the hard cap.
#                            Use this from P1 onward (per ADR §4.P0/P1).
#
# Waivers: a file may carry `//nolint:filesize` on its package-level doc
# comment to opt out, but must include a tracking issue and a target
# removal date per ADR D-11.

set -euo pipefail

SOFT=350
HARD=700
MODE="report-only"

while [ $# -gt 0 ]; do
  case "$1" in
    --report-only) MODE="report-only"; shift ;;
    --blocking)    MODE="blocking"; shift ;;
    --soft)        SOFT="$2"; shift 2 ;;
    --hard)        HARD="$2"; shift 2 ;;
    -h|--help)
      sed -n '3,22p' "$0"
      exit 0
      ;;
    *) echo "unknown flag: $1" >&2; exit 2 ;;
  esac
done

# Files to scan: every .go that is not a _test.go, not under .git, .claude,
# vendor, or examples. Examples are intentionally exempt — they exist to
# demonstrate usage, not to be production-grade.
mapfile -t FILES < <(
  find . \
    -type f -name '*.go' \
    -not -name '*_test.go' \
    -not -path './.git/*' \
    -not -path './.claude/*' \
    -not -path './vendor/*' \
    -not -path './examples/*' \
    -print | sort
)

over_soft=()
over_hard=()
waived=()

for f in "${FILES[@]}"; do
  lines=$(wc -l < "$f" | tr -d ' ')
  if grep -q '//nolint:filesize' "$f"; then
    waived+=("${lines} ${f}")
    continue
  fi
  if [ "$lines" -gt "$HARD" ]; then
    over_hard+=("${lines} ${f}")
  elif [ "$lines" -gt "$SOFT" ]; then
    over_soft+=("${lines} ${f}")
  fi
done

print_table() {
  # $1 = label, $2..$N = "lines path" rows
  local label="$1"; shift
  # Sort descending by first field (numeric)
  printf '%s\n' "$@" | sort -rn | awk '{ printf "    %6d  %s\n", $1, $2 }'
}

echo "==================================================================="
echo "  ADR 0001 D-11 — File-Size Budget Check (${MODE} mode)"
echo "  Soft target: ${SOFT} lines    Hard cap: ${HARD} lines"
echo "  Excluded: *_test.go, ./.git, ./.claude, ./vendor, ./examples"
echo "==================================================================="
echo

if [ "${#over_hard[@]}" -eq 0 ] && [ "${#over_soft[@]}" -eq 0 ] && [ "${#waived[@]}" -eq 0 ]; then
  echo "OK — every production .go file is within the soft target (${SOFT} lines)."
  exit 0
fi

if [ "${#over_hard[@]}" -gt 0 ]; then
  echo ">>> OVER HARD CAP (${HARD} lines)"
  echo "    These files MUST be split. See ADR §4 for planned phase."
  print_table "hard" "${over_hard[@]}"
  echo
fi

if [ "${#over_soft[@]}" -gt 0 ]; then
  echo ">>> OVER SOFT TARGET (${SOFT} lines) but within hard cap"
  echo "    Consider splitting when convenient; not blocking."
  print_table "soft" "${over_soft[@]}"
  echo
fi

if [ "${#waived[@]}" -gt 0 ]; then
  echo ">>> WAIVED (//nolint:filesize present)"
  print_table "waived" "${waived[@]}"
  echo
fi

if [ "$MODE" = "blocking" ] && [ "${#over_hard[@]}" -gt 0 ]; then
  echo "FAIL: ${#over_hard[@]} file(s) exceed the hard cap (${HARD} lines)." >&2
  exit 1
fi

echo "Report-only mode — no failure raised."
echo "Flip to --blocking from P1 onward (per ADR D-11)."
exit 0
