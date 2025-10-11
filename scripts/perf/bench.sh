#!/usr/bin/env bash

# bench.sh - Run benchmarks across all packages with standardized flags
# Usage: ./scripts/perf/bench.sh [count]
#   count: Number of times to run each benchmark (default: 5)

set -euo pipefail

# Configuration
COUNT="${1:-5}"
BENCHTIME="${BENCHTIME:-3s}"
ARTIFACTS_DIR="artifacts/bench"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}  Redis In-Memory Replica - Benchmark Suite${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo ""
echo -e "Configuration:"
echo -e "  Count:         ${GREEN}${COUNT}${NC}"
echo -e "  Bench time:    ${GREEN}${BENCHTIME}${NC}"
echo -e "  Output dir:    ${GREEN}${ARTIFACTS_DIR}${NC}"
echo -e "  GOMAXPROCS:    ${GREEN}${GOMAXPROCS:-default}${NC}"
echo ""

# Create artifacts directory
mkdir -p "${ARTIFACTS_DIR}"

# List of packages to benchmark
PACKAGES=(
    "./storage"
    "./lua"
    "./protocol"
    "./replication"
    "."
)

# Run benchmarks for each package
echo -e "${BLUE}Running benchmarks...${NC}"
echo ""

TOTAL=0
SUCCEEDED=0
FAILED=0

for pkg in "${PACKAGES[@]}"; do
    # Get package name for file naming
    pkg_name=$(basename "${pkg}")
    if [ "${pkg_name}" = "." ]; then
        pkg_name="root"
    fi
    
    output_file="${ARTIFACTS_DIR}/${pkg_name}_bench.txt"
    
    echo -e "${YELLOW}→${NC} Package: ${pkg}"
    
    # Check if package has benchmarks (use temp file to avoid pipe issues)
    tmpfile=$(mktemp)
    go test -list='Benchmark.*' "${pkg}" > "${tmpfile}" 2>&1
    if ! grep -q "^Benchmark" "${tmpfile}"; then
        rm -f "${tmpfile}"
        echo -e "  ${YELLOW}⚠${NC}  No benchmarks found, skipping"
        echo ""
        continue
    fi
    rm -f "${tmpfile}"
    
    TOTAL=$((TOTAL + 1))
    
    # Run benchmarks
    if go test -bench=. -run=^$ -benchmem -count="${COUNT}" -benchtime="${BENCHTIME}" "${pkg}" > "${output_file}" 2>&1; then
        # Extract summary
        bench_count=$(grep -c "^Benchmark" "${output_file}" || echo "0")
        echo -e "  ${GREEN}✓${NC}  Completed: ${bench_count} benchmarks"
        echo -e "  ${GREEN}→${NC}  Output: ${output_file}"
        SUCCEEDED=$((SUCCEEDED + 1))
    else
        echo -e "  ${RED}✗${NC}  Failed"
        echo -e "  ${RED}→${NC}  See: ${output_file}"
        FAILED=$((FAILED + 1))
    fi
    echo ""
done

# Create a combined output file with timestamp
combined_file="${ARTIFACTS_DIR}/all_benches_${TIMESTAMP}.txt"
{
    echo "# Combined Benchmark Results"
    echo "# Generated: $(date)"
    echo "# Count: ${COUNT}, Benchtime: ${BENCHTIME}"
    echo ""
    
    for pkg in "${PACKAGES[@]}"; do
        pkg_name=$(basename "${pkg}")
        if [ "${pkg_name}" = "." ]; then
            pkg_name="root"
        fi
        
        output_file="${ARTIFACTS_DIR}/${pkg_name}_bench.txt"
        if [ -f "${output_file}" ]; then
            echo "# ============================================"
            echo "# Package: ${pkg}"
            echo "# ============================================"
            cat "${output_file}"
            echo ""
        fi
    done
} > "${combined_file}"

echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}  Summary${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo ""
echo -e "  Packages tested:  ${TOTAL}"
echo -e "  ${GREEN}✓ Succeeded:      ${SUCCEEDED}${NC}"
if [ "${FAILED}" -gt 0 ]; then
    echo -e "  ${RED}✗ Failed:         ${FAILED}${NC}"
fi
echo ""
echo -e "  Results saved to:"
echo -e "    ${GREEN}${ARTIFACTS_DIR}/${NC}"
echo ""
echo -e "  Combined output:"
echo -e "    ${GREEN}${combined_file}${NC}"
echo ""

# Create latest symlink for easy reference
ln -sf "$(basename "${combined_file}")" "${ARTIFACTS_DIR}/latest.txt"

echo -e "${BLUE}Next steps:${NC}"
echo -e "  • Review results:    cat ${ARTIFACTS_DIR}/latest.txt"
echo -e "  • Compare with base: ./scripts/perf/compare.sh baseline.txt ${ARTIFACTS_DIR}/latest.txt"
echo -e "  • Profile hotspots:  ./scripts/perf/profile.sh <package> <benchmark>"
echo ""

exit 0
