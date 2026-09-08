#!/usr/bin/env bash
set -euo pipefail

# Protonman Comprehensive Coverage Runner (Unit + E2E Subprocesses)
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${REPO_ROOT}"

COVER_DIR="/tmp/proton_e2e_cov_$(date +%s)"
UNIT_OUT="/tmp/proton_unit_$(date +%s).out"
MERGED_OUT="${REPO_ROOT}/coverage.out"

mkdir -p "${COVER_DIR}"

echo "================================================="
echo "Running Unit Tests with Coverage..."
echo "================================================="
go test -count=1 -coverprofile="${UNIT_OUT}" -covermode=set ./...

echo "================================================="
echo "Running Comprehensive E2E Tests with Subprocess Coverage..."
echo "================================================="
PROTON_COVERDIR="${COVER_DIR}" PROTON_E2E_COVERAGE=1 go test -count=1 -v ./test/e2e/...

echo "================================================="
echo "Subprocess Package Coverage Breakdown:"
echo "================================================="
go tool covdata percent -i="${COVER_DIR}"

echo "================================================="
echo "Generating Merged Coverage Profile..."
echo "================================================="
go tool covdata textfmt -i="${COVER_DIR}" -o="${MERGED_OUT}"

if command -v go tool cover >/dev/null 2>&1; then
    echo "Overall Statement Coverage Report:"
    go tool cover -func="${MERGED_OUT}" | tail -n 1
fi

echo "================================================="
echo "Coverage artifacts successfully generated!"
echo "Profile: ${MERGED_OUT}"
echo "================================================="
