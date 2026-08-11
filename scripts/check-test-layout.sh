#!/usr/bin/env bash
# check-test-layout.sh — Kiểm tra test file không nằm cạnh source code.
#
# Quy tắc:
# 1. Backend: Không có file *_test.go nào nằm ngoài backend/tests/ (trừ backend/tests/ chính nó).
# 2. Frontend: Không có file *.test.ts nào nằm ngoài frontend/tests/.
# 3. Root-level: Không có file *.js hay *.py test nào nằm ngoài tests/ (k6/monitoring).
#
# Chạy: scripts/check-test-layout.sh
# Exit code: 0 nếu hợp lệ, 1 nếu có vi phạm.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HAS_ERROR=0

# Màu cho output
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

# ── Kết luận ──────────────────────────────────────────────────────────────────
echo ""
if [ "$HAS_ERROR" -eq 0 ]; then
  echo -e "${GREEN}✅ Test layout is clean.${NC}"
else
  echo -e "${RED}❌ Test layout violations found. Move test files into the correct tests/ directory.${NC}"
fi
exit "$HAS_ERROR"