#!/usr/bin/env bash

# compare.sh - Compare two benchmark results using benchstat
# Usage: ./scripts/perf/compare.sh <baseline.txt> <current.txt>

set -euo pipefail

# Check arguments
if [ $# -lt 2 ]; then
    echo "Usage: $0 <baseline.txt> <current.txt>"
    echo ""
    echo "Example:"
    echo "  $0 artifacts/bench/baseline.txt artifacts/bench/current.txt"
    echo ""
    echo "Note: benchstat must be installed. Install with:"
    echo "  go install golang.org/x/perf/cmd/benchstat@latest"
    exit 1
fi

BASELINE="$1"
CURRENT="$2"
ARTIFACTS_DIR="artifacts/bench"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}  Redis In-Memory Replica - Benchmark Comparison${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo ""
echo -e "Configuration:"
echo -e "  Baseline:  ${GREEN}${BASELINE}${NC}"
echo -e "  Current:   ${GREEN}${CURRENT}${NC}"
echo ""

# Check if files exist
if [ ! -f "${BASELINE}" ]; then
    echo -e "${RED}✗${NC} Baseline file not found: ${BASELINE}"
    exit 1
fi

if [ ! -f "${CURRENT}" ]; then
    echo -e "${RED}✗${NC} Current file not found: ${CURRENT}"
    exit 1
fi

# Check if benchstat is installed
if ! command -v benchstat >/dev/null 2>&1; then
    echo -e "${YELLOW}⚠${NC}  benchstat is not installed"
    echo ""
    echo "benchstat is optional but recommended for statistical comparison."
    echo ""
    echo "To install:"
    echo -e "  ${GREEN}go install golang.org/x/perf/cmd/benchstat@latest${NC}"
    echo ""
    echo "Without benchstat, you can still manually compare the files:"
    echo -e "  ${GREEN}diff ${BASELINE} ${CURRENT}${NC}"
    echo ""
    exit 0
fi

# Create artifacts directory
mkdir -p "${ARTIFACTS_DIR}"

# Output file
OUTPUT_FILE="${ARTIFACTS_DIR}/benchstat_${TIMESTAMP}.diff"

echo -e "${YELLOW}→${NC} Running benchstat comparison..."
echo ""

# Run benchstat and save output
if benchstat "${BASELINE}" "${CURRENT}" | tee "${OUTPUT_FILE}"; then
    echo ""
    echo -e "${GREEN}✓${NC} Comparison completed"
    echo -e "${GREEN}→${NC} Full output saved to: ${OUTPUT_FILE}"
    
    # Create latest symlink
    ln -sf "$(basename "${OUTPUT_FILE}")" "${ARTIFACTS_DIR}/benchstat_latest.diff"
    
    # Analyze results for regressions
    echo ""
    echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
    echo -e "${BLUE}  Analysis${NC}"
    echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
    echo ""
    
    # Check for significant performance changes
    if grep -q "~" "${OUTPUT_FILE}"; then
        echo -e "${GREEN}ℹ${NC}  Found benchmarks with no significant change (~)"
    fi
    
    # Count improvements and regressions
    improvements=$(grep -c "faster" "${OUTPUT_FILE}" || echo "0")
    regressions=$(grep -c "slower" "${OUTPUT_FILE}" || echo "0")
    
    if [ "${improvements}" -gt 0 ]; then
        echo -e "${GREEN}✓${NC}  Performance improvements: ${improvements}"
    fi
    
    if [ "${regressions}" -gt 0 ]; then
        echo -e "${RED}⚠${NC}  Performance regressions: ${regressions}"
        echo ""
        echo -e "${YELLOW}Regressions detected:${NC}"
        grep "slower" "${OUTPUT_FILE}" || true
    fi
    
    # Check for allocation changes
    alloc_increases=$(grep -c "B/op.*+" "${OUTPUT_FILE}" || echo "0")
    if [ "${alloc_increases}" -gt 0 ]; then
        echo -e "${YELLOW}⚠${NC}  Allocation increases detected: ${alloc_increases}"
    fi
    
else
    echo -e "${RED}✗${NC} benchstat failed"
    exit 1
fi

echo ""
echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}  Interpretation Guide${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo ""
echo "Understanding the results:"
echo ""
echo "  • delta      - Percentage change (negative is improvement for time)"
echo "  • p-value    - Statistical significance (p < 0.05 is significant)"
echo "  • ±          - Confidence interval"
echo "  • ~          - No significant change detected"
echo ""
echo "Guidelines:"
echo "  • > 20% slower: Investigate immediately"
echo "  • > 50% more allocations: Check for new allocations"
echo "  • New allocs in 0-alloc paths: Likely a regression"
echo ""
echo "For detailed analysis:"
echo -e "  ${GREEN}cat ${OUTPUT_FILE}${NC}"
echo ""

exit 0
