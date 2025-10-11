#!/usr/bin/env bash

# profile.sh - Generate CPU, memory, or allocation profiles for a specific benchmark
# Usage: ./scripts/perf/profile.sh <package> <bench_regex> [profile_type]
#   package:      Package path (e.g., ./storage)
#   bench_regex:  Benchmark name pattern (e.g., BenchmarkStorageSet)
#   profile_type: cpu, mem, or allocs (default: cpu)

set -euo pipefail

# Check arguments
if [ $# -lt 2 ]; then
    echo "Usage: $0 <package> <bench_regex> [profile_type]"
    echo ""
    echo "Examples:"
    echo "  $0 ./storage BenchmarkStorageSet cpu"
    echo "  $0 ./storage BenchmarkStorageGet mem"
    echo "  $0 ./lua BenchmarkLuaEngine_SimpleScript allocs"
    echo ""
    echo "Profile types: cpu, mem, allocs"
    exit 1
fi

PACKAGE="$1"
BENCH_REGEX="$2"
PROFILE_TYPE="${3:-cpu}"
ARTIFACTS_DIR="artifacts/profiles"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}  Redis In-Memory Replica - Profiler${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo ""
echo -e "Configuration:"
echo -e "  Package:       ${GREEN}${PACKAGE}${NC}"
echo -e "  Benchmark:     ${GREEN}${BENCH_REGEX}${NC}"
echo -e "  Profile type:  ${GREEN}${PROFILE_TYPE}${NC}"
echo ""

# Create artifacts directory
mkdir -p "${ARTIFACTS_DIR}"

# Sanitize benchmark name for filename
safe_bench_name=$(echo "${BENCH_REGEX}" | tr '/' '_' | tr '^$*' '_')
pkg_name=$(basename "${PACKAGE}")
if [ "${pkg_name}" = "." ]; then
    pkg_name="root"
fi

# Generate profile based on type
case "${PROFILE_TYPE}" in
    cpu)
        PROFILE_FILE="${ARTIFACTS_DIR}/${pkg_name}_${safe_bench_name}_cpu_${TIMESTAMP}.prof"
        echo -e "${YELLOW}→${NC} Generating CPU profile..."
        
        if go test -bench="${BENCH_REGEX}" -run=^$ -cpuprofile="${PROFILE_FILE}" "${PACKAGE}" > /dev/null 2>&1; then
            echo -e "${GREEN}✓${NC} CPU profile generated: ${PROFILE_FILE}"
            
            # Generate text summary
            echo ""
            echo -e "${YELLOW}→${NC} Top 20 functions by CPU time:"
            go tool pprof -text -nodecount=20 "${PROFILE_FILE}" 2>/dev/null || echo "Unable to generate text summary"
            
            # Try to generate SVG if pprof supports it
            SVG_FILE="${ARTIFACTS_DIR}/${pkg_name}_${safe_bench_name}_cpu_${TIMESTAMP}.svg"
            if command -v dot >/dev/null 2>&1; then
                echo ""
                echo -e "${YELLOW}→${NC} Generating SVG graph..."
                if go tool pprof -svg "${PROFILE_FILE}" > "${SVG_FILE}" 2>/dev/null; then
                    echo -e "${GREEN}✓${NC} SVG graph: ${SVG_FILE}"
                fi
            else
                echo -e "${YELLOW}ℹ${NC}  Install graphviz for SVG generation: sudo apt-get install graphviz"
            fi
        else
            echo -e "${RED}✗${NC} Failed to generate CPU profile"
            exit 1
        fi
        ;;
        
    mem)
        PROFILE_FILE="${ARTIFACTS_DIR}/${pkg_name}_${safe_bench_name}_mem_${TIMESTAMP}.prof"
        echo -e "${YELLOW}→${NC} Generating memory profile..."
        
        if go test -bench="${BENCH_REGEX}" -run=^$ -memprofile="${PROFILE_FILE}" "${PACKAGE}" > /dev/null 2>&1; then
            echo -e "${GREEN}✓${NC} Memory profile generated: ${PROFILE_FILE}"
            
            # Generate text summaries
            echo ""
            echo -e "${YELLOW}→${NC} Top 20 functions by allocated bytes:"
            go tool pprof -text -nodecount=20 -alloc_space "${PROFILE_FILE}" 2>/dev/null || echo "Unable to generate text summary"
            
            echo ""
            echo -e "${YELLOW}→${NC} Top 20 functions by in-use memory:"
            go tool pprof -text -nodecount=20 -inuse_space "${PROFILE_FILE}" 2>/dev/null || echo "Unable to generate text summary"
            
            # Try to generate SVG
            SVG_FILE="${ARTIFACTS_DIR}/${pkg_name}_${safe_bench_name}_mem_${TIMESTAMP}.svg"
            if command -v dot >/dev/null 2>&1; then
                echo ""
                echo -e "${YELLOW}→${NC} Generating SVG graph..."
                if go tool pprof -svg -alloc_space "${PROFILE_FILE}" > "${SVG_FILE}" 2>/dev/null; then
                    echo -e "${GREEN}✓${NC} SVG graph: ${SVG_FILE}"
                fi
            fi
        else
            echo -e "${RED}✗${NC} Failed to generate memory profile"
            exit 1
        fi
        ;;
        
    allocs)
        PROFILE_FILE="${ARTIFACTS_DIR}/${pkg_name}_${safe_bench_name}_allocs_${TIMESTAMP}.prof"
        echo -e "${YELLOW}→${NC} Generating allocation profile..."
        
        if go test -bench="${BENCH_REGEX}" -run=^$ -memprofile="${PROFILE_FILE}" -memprofilerate=1 "${PACKAGE}" > /dev/null 2>&1; then
            echo -e "${GREEN}✓${NC} Allocation profile generated: ${PROFILE_FILE}"
            
            # Generate text summaries
            echo ""
            echo -e "${YELLOW}→${NC} Top 20 functions by allocation count:"
            go tool pprof -text -nodecount=20 -alloc_objects "${PROFILE_FILE}" 2>/dev/null || echo "Unable to generate text summary"
            
            echo ""
            echo -e "${YELLOW}→${NC} Top 20 functions by allocated bytes:"
            go tool pprof -text -nodecount=20 -alloc_space "${PROFILE_FILE}" 2>/dev/null || echo "Unable to generate text summary"
            
            # Try to generate SVG
            SVG_FILE="${ARTIFACTS_DIR}/${pkg_name}_${safe_bench_name}_allocs_${TIMESTAMP}.svg"
            if command -v dot >/dev/null 2>&1; then
                echo ""
                echo -e "${YELLOW}→${NC} Generating SVG graph..."
                if go tool pprof -svg -alloc_objects "${PROFILE_FILE}" > "${SVG_FILE}" 2>/dev/null; then
                    echo -e "${GREEN}✓${NC} SVG graph: ${SVG_FILE}"
                fi
            fi
        else
            echo -e "${RED}✗${NC} Failed to generate allocation profile"
            exit 1
        fi
        ;;
        
    *)
        echo -e "${RED}✗${NC} Unknown profile type: ${PROFILE_TYPE}"
        echo "Valid types: cpu, mem, allocs"
        exit 1
        ;;
esac

echo ""
echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}  Profile Generation Complete${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo ""
echo -e "${BLUE}Next steps:${NC}"
echo -e "  • Interactive analysis: go tool pprof ${PROFILE_FILE}"
echo -e "  • Web interface:       go tool pprof -http=:8080 ${PROFILE_FILE}"
echo -e "  • Text report:         go tool pprof -text ${PROFILE_FILE}"
echo -e "  • See all profiles:    ls -lh ${ARTIFACTS_DIR}/"
echo ""

exit 0
