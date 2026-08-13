#!/usr/bin/env bash
# check-test-layout.sh — Check that test files are not placed next to source code.
#
# Rules:
# 1. Backend: No *_test.go files outside backend/tests/ (except backend/tests/ itself).
# 2. Frontend: No *.test.ts files outside frontend/tests/.
# 3. Root-level: No *.js or *.py test files outside tests/ (k6/monitoring).
#
# Run: scripts/check-test-layout.sh
# Exit code: 0 if valid, 1 if violations found.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HAS_ERROR=0

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

# ── Backend: Go test files ────────────────────────────────────────────────────
echo "🔍 Backend: checking Go test files..."
while IFS= read -r -d '' f; do
  rel="${f#$ROOT/}"
  if [[ "$rel" != backend/tests/* ]]; then
    echo -e "  ${RED}✗${NC} $rel"
    HAS_ERROR=1
  fi
done < <(find "$ROOT/backend" -name '*_test.go' -not -path '*/tests/*' -print0 2>/dev/null)

if [ "$HAS_ERROR" -eq 0 ]; then
  echo -e "  ${GREEN}✓${NC} No backend test files outside backend/tests/"
fi

# ── Frontend: TypeScript test files ───────────────────────────────────────────
echo "🔍 Frontend: checking TypeScript test files..."
while IFS= read -r -d '' f; do
  rel="${f#$ROOT/}"
  if [[ "$rel" != frontend/tests/* ]]; then
    echo -e "  ${RED}✗${NC} $rel"
    HAS_ERROR=1
  fi
done < <(find "$ROOT/frontend" -name '*.test.ts' -not -path '*/tests/*' -not -path '*/node_modules/*' -print0 2>/dev/null)

if [ "$HAS_ERROR" -eq 0 ]; then
  echo -e "  ${GREEN}✓${NC} No frontend test files outside frontend/tests/"
fi

# ── Root-level: k6/monitoring test files ──────────────────────────────────────
echo "🔍 Root-level: checking test scripts..."
while IFS= read -r -d '' f; do
  rel="${f#$ROOT/}"
  if [[ "$rel" != tests/* ]]; then
    echo -e "  ${RED}✗${NC} $rel"
    HAS_ERROR=1
  fi
done < <(find "$ROOT" -maxdepth 1 \( -name '*.js' -o -name '*.py' \) -print0 2>/dev/null)

if [ "$HAS_ERROR" -eq 0 ]; then
  echo -e "  ${GREEN}✓${NC} No test scripts at repo root (all in tests/)"
fi

# ── Conclusion ─────────────────────────────────────────────────────────────────
echo ""
if [ "$HAS_ERROR" -eq 0 ]; then
  echo -e "${GREEN}✅ Test layout is clean.${NC}"
else
  echo -e "${RED}❌ Test layout violations found. Move test files into the correct tests/ directory.${NC}"
fi
exit "$HAS_ERROR"