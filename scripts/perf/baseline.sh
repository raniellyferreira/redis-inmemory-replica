#!/usr/bin/env bash
#
# baseline.sh — capture a complete performance baseline snapshot per
# ADR 0001 D-10 §10.4: benchmarks (benchstat-ready), escape-analysis
# text for hot files, and a record of the toolchain/runner state.
#
# Output: artifacts/baseline/<TIMESTAMP>/
#   - bench-storage.txt         go test -bench results for storage
#   - bench-protocol.txt        ... for protocol
#   - bench-lua.txt             ... for lua
#   - bench-replication.txt     ... for replication
#   - bench-root.txt            ... for the root package (DatabaseInfo benches;
#                                replication benches skip without Redis)
#   - bench-all.txt             concatenation of the above, ready for benchstat
#   - escape-analysis.txt       go build -gcflags=-m output for hot packages
#   - env.txt                   go version, GOMAXPROCS, kernel, CPU, memory
#
# Usage:
#   ./scripts/perf/baseline.sh           # default: count=5 benchtime=3s
#   ./scripts/perf/baseline.sh 3 2s      # count=3 benchtime=2s (quick)
#
# Recommended invocation from CI / release-prep is `make baseline`.
# Re-running this OVERWRITES the latest symlink but never deletes prior
# timestamped snapshots — keep all historical baselines for benchstat.

set -euo pipefail

COUNT="${1:-5}"
BENCHTIME="${2:-${BENCHTIME:-3s}}"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
OUT_BASE="artifacts/baseline"
OUT_DIR="${OUT_BASE}/${TIMESTAMP}"

mkdir -p "${OUT_DIR}"

echo "==================================================================="
echo "  Baseline snapshot — ${TIMESTAMP}"
echo "  count=${COUNT}, benchtime=${BENCHTIME}, GOMAXPROCS=${GOMAXPROCS:-default}"
echo "==================================================================="

# Capture environment
{
  echo "## Environment"
  echo "Date:        $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "Hostname:    $(hostname 2>/dev/null || echo n/a)"
  echo "OS:          $(uname -srm)"
  echo "Go version:  $(go version)"
  echo "GOMAXPROCS:  ${GOMAXPROCS:-default}"
  echo "CPU:         $(grep -m1 'model name' /proc/cpuinfo 2>/dev/null | sed 's/.*: //' || echo n/a)"
  echo "Cores:       $(nproc 2>/dev/null || echo n/a)"
  echo "Memory:      $(grep MemTotal /proc/meminfo 2>/dev/null | awk '{print $2/1024 " MB"}' || echo n/a)"
  echo "Git commit:  $(git rev-parse HEAD 2>/dev/null || echo n/a)"
  echo "Git status:  $(git diff --stat 2>/dev/null | tail -1 || echo n/a)"
} > "${OUT_DIR}/env.txt"
cat "${OUT_DIR}/env.txt"
echo

# Benchmark each production package separately so a flaky/slow one doesn't
# torpedo the rest. We collect everything; presence of failures is visible
# from the per-file content, not from this script's exit code.
PKGS=(
  "storage:./storage/"
  "protocol:./protocol/"
  "lua:./lua/"
  "replication:./replication/"
  "root:."
)

for entry in "${PKGS[@]}"; do
  label="${entry%%:*}"
  pkg="${entry#*:}"
  out="${OUT_DIR}/bench-${label}.txt"
  echo "→ Benchmarking ${pkg}  (${label})"
  if go test -bench=. -run=^$ -benchmem -count="${COUNT}" -benchtime="${BENCHTIME}" "${pkg}" \
       > "${out}" 2>&1; then
    n=$(grep -c '^Benchmark' "${out}" || true)
    echo "  ✓ ${n} measurement lines → ${out}"
  else
    echo "  ✗ failed — see ${out}"
  fi
done

# Concatenate into a single benchstat-ready file
cat "${OUT_DIR}"/bench-*.txt > "${OUT_DIR}/bench-all.txt"
echo
echo "→ Combined: ${OUT_DIR}/bench-all.txt ($(grep -c '^Benchmark' "${OUT_DIR}/bench-all.txt" || true) measurement lines)"

# Escape analysis on hot files — `-gcflags="-m=2"` emits inlining and
# escape decisions. We capture the WHOLE build output, then store it for
# later diffing by `make escape-analysis` (which is the diff target).
echo
echo "→ Capturing escape analysis for hot packages"
go build -gcflags="-m=2" ./storage/... ./protocol/... ./lua/... ./replication/... ./server/... ./ \
  > "${OUT_DIR}/escape-analysis.txt" 2>&1 || true
echo "  ✓ $(wc -l < "${OUT_DIR}/escape-analysis.txt") lines → ${OUT_DIR}/escape-analysis.txt"

# Symlink latest for convenience
ln -sfn "${TIMESTAMP}" "${OUT_BASE}/latest"
echo
echo "==================================================================="
echo "  Baseline captured at ${OUT_DIR}"
echo "  Latest symlink: ${OUT_BASE}/latest -> ${TIMESTAMP}"
echo "==================================================================="
echo
echo "Next steps:"
echo "  - Compare a current run: make bench-compare base=${OUT_DIR}/bench-all.txt head=current.txt"
echo "  - Diff escape analysis : make escape-analysis (diffs current vs ${OUT_BASE}/latest)"
echo
