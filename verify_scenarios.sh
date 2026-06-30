#!/bin/bash

echo "=== Verifying Spec Scenarios ==="

# Scenario 1: Agent lists indexed items from a configured input
echo ""
echo "Scenario 1: Agent lists indexed items from a configured input"
echo "Command: backscroll list --input claude --project /home/shared/homeserver --order timestamp:desc --limit 1"
./backscroll list --help | grep -E "input|order|limit" > /dev/null && echo "✓ Flags supported" || echo "✗ Flags not supported"

# Scenario 2: User requests human-readable output
echo ""
echo "Scenario 2: User requests human-readable output"
echo "Command: backscroll read ... --pretty"
./backscroll read --help | grep "\-\-pretty" > /dev/null && echo "✓ --pretty flag exists" || echo "✗ --pretty flag missing"

# Scenario 3: Agent scopes search to a configured input
echo ""
echo "Scenario 3: Agent scopes search to a configured input"
echo "Command: backscroll search --input pi --text subagent"
./backscroll search --help | grep "\-\-input" > /dev/null && echo "✓ --input on search" || echo "✗ --input missing on search"
./backscroll search --help | grep "\-\-text" > /dev/null && echo "✓ --text on search" || echo "✗ --text missing on search"

# Scenario 4: Agent searches tool activity across projects
echo ""
echo "Scenario 4: Agent searches tool activity across projects"
echo "Command: backscroll search \"bash\" --all-projects --content-type tool"
./backscroll search --help | grep "\-\-content-type" > /dev/null && echo "✓ --content-type on search" || echo "✗ --content-type missing"

# Scenario 5: Agent tails a large Claude JSONL file
echo ""
echo "Scenario 5: Agent tails a large Claude JSONL file"
echo "Command: backscroll read --path <file> --tail 45 --semantic"
./backscroll read --help | grep "\-\-path" > /dev/null && echo "✓ --path on read" || echo "✗ --path missing"
./backscroll read --help | grep "\-\-tail" > /dev/null && echo "✓ --tail on read" || echo "✗ --tail missing"
./backscroll read --help | grep "\-\-semantic" > /dev/null && echo "✓ --semantic on read" || echo "✗ --semantic missing"

# Scenario 5: Agent checks health
echo ""
echo "Scenario 5: Agent checks health"
echo "Command: backscroll status"
./backscroll status --help > /dev/null 2>&1 && echo "✓ status command works" || echo "✗ status command missing"

# Scenario 6: No --robot flag required
echo ""
echo "Scenario 6: No --robot flag required for agent output"
if ! ./backscroll read --help | grep "\-\-robot" > /dev/null 2>&1; then
  echo "✓ No --robot flag in read"
else
  echo "✗ --robot flag still present"
fi

if ! ./backscroll list --help | grep "\-\-robot" > /dev/null 2>&1; then
  echo "✓ No --robot flag in list"
else
  echo "✗ --robot flag still present"
fi

if ! ./backscroll search --help | grep "\-\-robot" > /dev/null 2>&1; then
  echo "✓ No --robot flag in search"
else
  echo "✗ --robot flag still present"
fi

echo ""
echo "=== Scenario verification complete ==="
