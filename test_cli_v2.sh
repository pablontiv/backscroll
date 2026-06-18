#!/bin/bash
set -e

echo "=== Testing v2 CLI Surface ==="

# Test 1: root help shows only v2 commands
echo "Test 1: Root help shows v2 commands"
./backscroll --help | grep -E "^\s+list|^\s+search|^\s+read|^\s+stats|^\s+status|^\s+validate|^\s+rebuild|^\s+purge|^\s+config" | wc -l
CMDCOUNT=$(./backscroll --help | grep -E "^\s+list|^\s+search|^\s+read|^\s+stats|^\s+status|^\s+validate|^\s+rebuild|^\s+purge|^\s+config" | wc -l)
if [ "$CMDCOUNT" -eq 9 ]; then
  echo "✓ All 9 v2 commands present in root help"
else
  echo "✗ Expected 9 v2 commands, got $CMDCOUNT"
fi

# Test 2: Verify v1 commands are NOT in help
echo ""
echo "Test 2: v1 commands removed from help"
if ! ./backscroll --help | grep -E "^\s+(sessions|events|inputs|projects|topics|insights|export|sync|resume|reindex)" 2>/dev/null; then
  echo "✓ No v1 commands in root help"
else
  echo "✗ Found v1 commands in help"
fi

# Test 3: read --path flag exists
echo ""
echo "Test 3: read --path flag"
./backscroll read --help 2>&1 | grep -q "\-\-path" && echo "✓ read --path flag exists" || echo "✗ read --path flag missing"

# Test 4: read --tail flag exists
echo ""
echo "Test 4: read --tail flag"
./backscroll read --help 2>&1 | grep -q "\-\-tail" && echo "✓ read --tail flag exists" || echo "✗ read --tail flag missing"

# Test 5: read --semantic flag exists
echo ""
echo "Test 5: read --semantic flag"
./backscroll read --help 2>&1 | grep -q "\-\-semantic" && echo "✓ read --semantic flag exists" || echo "✗ read --semantic flag missing"

# Test 6: read --pretty flag exists
echo ""
echo "Test 6: read --pretty flag"
./backscroll read --help 2>&1 | grep -q "\-\-pretty" && echo "✓ read --pretty flag exists" || echo "✗ read --pretty flag missing"

# Test 7: list --input flag exists
echo ""
echo "Test 7: list --input flag"
./backscroll list --help 2>&1 | grep -q "\-\-input" && echo "✓ list --input flag exists" || echo "✗ list --input flag missing"

# Test 8: list --order flag exists
echo ""
echo "Test 8: list --order flag"
./backscroll list --help 2>&1 | grep -q "\-\-order" && echo "✓ list --order flag exists" || echo "✗ list --order flag missing"

# Test 9: search --text flag exists
echo ""
echo "Test 9: search --text flag"
./backscroll search --help 2>&1 | grep -q "\-\-text" && echo "✓ search --text flag exists" || echo "✗ search --text flag missing"

# Test 10: stats command exists
echo ""
echo "Test 10: stats command"
./backscroll stats --help 2>&1 | grep -q "Aggregate statistics" && echo "✓ stats command exists" || echo "✗ stats command missing"

# Test 11: stats --group-by flag
echo ""
echo "Test 11: stats --group-by flag"
./backscroll stats --help 2>&1 | grep -q "\-\-group-by" && echo "✓ stats --group-by flag exists" || echo "✗ stats --group-by flag missing"

# Test 12: config command exists
echo ""
echo "Test 12: config command"
./backscroll config --help 2>&1 | grep -q "Show effective configuration" && echo "✓ config command exists" || echo "✗ config command missing"

# Test 13: rebuild command exists
echo ""
echo "Test 13: rebuild command"
./backscroll rebuild --help 2>&1 | grep -q "Clear and rebuild" && echo "✓ rebuild command exists" || echo "✗ rebuild command missing"

# Test 14: validate --indexed-only flag
echo ""
echo "Test 14: validate --indexed-only flag"
./backscroll validate --help 2>&1 | grep -q "\-\-indexed-only" && echo "✓ validate --indexed-only flag exists" || echo "✗ validate --indexed-only flag missing"

# Test 15: search --indexed-only flag
echo ""
echo "Test 15: search --indexed-only flag"
./backscroll search --help 2>&1 | grep -q "\-\-indexed-only" && echo "✓ search --indexed-only flag exists" || echo "✗ search --indexed-only flag missing"

# Test 16: Verify no --robot flag on read
echo ""
echo "Test 16: no --robot flag on v2 read"
if ! ./backscroll read --help 2>&1 | grep -w "\-\-robot" 2>/dev/null; then
  echo "✓ read command does not expose --robot flag"
else
  echo "✗ read command still has --robot flag"
fi

echo ""
echo "=== All CLI surface tests completed ==="
