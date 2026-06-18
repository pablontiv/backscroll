#!/bin/bash
set -e

echo "=== Final Acceptance Checks ==="

# Check 1: just test passes
echo ""
echo "Check 1: just test passes"
if just test > /tmp/test.log 2>&1; then
  echo "✓ PASS: just test"
  RESULT1="PASS"
else
  echo "✗ FAIL: just test"
  RESULT1="FAIL"
  tail -20 /tmp/test.log
fi

# Check 2: just check passes
echo ""
echo "Check 2: just check passes"
if just check > /tmp/check.log 2>&1; then
  echo "✓ PASS: just check"
  RESULT2="PASS"
else
  echo "✗ FAIL: just check"
  RESULT2="FAIL"
  tail -20 /tmp/check.log
fi

# Check 3: just coverage-check passes
echo ""
echo "Check 3: just coverage-check passes"
if just coverage-check > /tmp/coverage.log 2>&1; then
  echo "✓ PASS: just coverage-check"
  RESULT3="PASS"
else
  echo "✗ FAIL: just coverage-check"
  RESULT3="FAIL"
  tail -20 /tmp/coverage.log
fi

# Check 4: Root help exposes v2 command surface only
echo ""
echo "Check 4: Root help exposes v2 command surface only"
if ./backscroll --help | grep -E "^\s+(list|search|read|stats|status|validate|rebuild|purge|config)\s" > /dev/null; then
  CMDCOUNT=$(./backscroll --help | grep -E "^\s+(list|search|read|stats|status|validate|rebuild|purge|config)\s" | wc -l)
  if [ "$CMDCOUNT" -eq 9 ]; then
    echo "✓ PASS: All 9 v2 commands in help"
    RESULT4="PASS"
  else
    echo "✗ FAIL: Expected 9, got $CMDCOUNT"
    RESULT4="FAIL"
  fi
else
  echo "✗ FAIL: v2 commands not found in help"
  RESULT4="FAIL"
fi

# Check 5: No v1 commands in help
echo ""
echo "Check 5: No v1 commands in help"
if ! ./backscroll --help | grep -E "^\s+(sessions|events|inputs|projects|topics|insights|export|sync|resume|reindex)\s"; then
  echo "✓ PASS: No v1 commands in help"
  RESULT5="PASS"
else
  echo "✗ FAIL: v1 commands found in help"
  RESULT5="FAIL"
fi

# Check 6: read --path flag works
echo ""
echo "Check 6: read --path flag works"
if ./backscroll read --help | grep -q "\-\-path"; then
  echo "✓ PASS: read --path exists"
  RESULT6="PASS"
else
  echo "✗ FAIL: read --path missing"
  RESULT6="FAIL"
fi

# Check 7: list --input flag works
echo ""
echo "Check 7: list --input flag works"
if ./backscroll list --help | grep -q "\-\-input"; then
  echo "✓ PASS: list --input exists"
  RESULT7="PASS"
else
  echo "✗ FAIL: list --input missing"
  RESULT7="FAIL"
fi

# Check 8: stats command exists
echo ""
echo "Check 8: stats command exists"
if ./backscroll stats --help > /dev/null 2>&1; then
  echo "✓ PASS: stats command exists"
  RESULT8="PASS"
else
  echo "✗ FAIL: stats command missing"
  RESULT8="FAIL"
fi

# Check 9: config command exists
echo ""
echo "Check 9: config command exists"
if ./backscroll config --help > /dev/null 2>&1; then
  echo "✓ PASS: config command exists"
  RESULT9="PASS"
else
  echo "✗ FAIL: config command missing"
  RESULT9="FAIL"
fi

# Check 10: rebuild command exists
echo ""
echo "Check 10: rebuild command exists"
if ./backscroll rebuild --help > /dev/null 2>&1; then
  echo "✓ PASS: rebuild command exists"
  RESULT10="PASS"
else
  echo "✗ FAIL: rebuild command missing"
  RESULT10="FAIL"
fi

echo ""
echo "=== Summary ==="
echo "just test: $RESULT1"
echo "just check: $RESULT2"
echo "just coverage-check: $RESULT3"
echo "v2 command surface: $RESULT4"
echo "no v1 commands: $RESULT5"
echo "read --path: $RESULT6"
echo "list --input: $RESULT7"
echo "stats command: $RESULT8"
echo "config command: $RESULT9"
echo "rebuild command: $RESULT10"

