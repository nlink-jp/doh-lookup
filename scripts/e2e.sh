#!/bin/sh
# e2e.sh — end-to-end smoke test driving the built doh-lookup binary against
# the real Cloudflare/Google DoH endpoints. Network is required. This is the
# "run the shipped binary against real data" gate (complements the tagged Go
# live tests in e2e/). Run: make e2e  (or scripts/e2e.sh).
#
# Exit 0 iff every check passes; non-zero (and a FAIL summary) otherwise.

set -u

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
BIN="$ROOT/dist/doh-lookup"

# Isolate the cache so a stale entry can never mask a regression.
CACHE_DIR=$(mktemp -d 2>/dev/null || echo "/tmp/doh-lookup-e2e.$$")
export DOH_LOOKUP_CACHE_DIR="$CACHE_DIR"
export XDG_CONFIG_HOME="$CACHE_DIR/cfg"   # keep the user's real config out
trap 'rm -rf "$CACHE_DIR"' EXIT

PASS=0
FAIL=0

if [ ! -x "$BIN" ]; then
  echo "[e2e] building $BIN ..."
  ( cd "$ROOT" && make build >/dev/null ) || { echo "[e2e] build failed"; exit 1; }
fi

# ok <desc> — the previous command block set OK=1/0 and OUT.
report() {
  if [ "$1" -eq 1 ]; then
    PASS=$((PASS + 1)); printf '  PASS  %s\n' "$2"
  else
    FAIL=$((FAIL + 1)); printf '  FAIL  %s\n' "$2"
    printf '%s\n' "$3" | sed 's/^/          /'
  fi
}

echo "[e2e] doh-lookup against live Cloudflare/Google DoH"

# 1. version
OUT=$("$BIN" version 2>&1); C=$?
echo "$OUT" | grep -q "doh-lookup" && [ $C -eq 0 ] && OK=1 || OK=0
report "$OK" "version prints and exits 0" "$OUT"

# 2. forward A, Cloudflare — provenance + an A address + DNSSEC validated
OUT=$("$BIN" lookup --refresh --type A example.com 2>&1); C=$?
if [ $C -eq 0 ] && echo "$OUT" | grep -q "via cloudflare" \
   && echo "$OUT" | grep -Eq '  A +example\.com' \
   && echo "$OUT" | grep -q "DNSSEC:validated"; then OK=1; else OK=0; fi
report "$OK" "forward A example.com via Cloudflare (provenance + A + DNSSEC)" "$OUT"

# 3. forward A, Google
OUT=$("$BIN" lookup --refresh --type A --provider google example.com 2>&1); C=$?
[ $C -eq 0 ] && echo "$OUT" | grep -q "via google" && OK=1 || OK=0
report "$OK" "forward A example.com via Google" "$OUT"

# 4. reverse PTR
OUT=$("$BIN" lookup --refresh 8.8.8.8 2>&1); C=$?
if [ $C -eq 0 ] && echo "$OUT" | grep -q "reverse" && echo "$OUT" | grep -q "dns.google"; then OK=1; else OK=0; fi
report "$OK" "reverse PTR 8.8.8.8 -> dns.google" "$OUT"

# 5. NXDOMAIN -> exit 1
OUT=$("$BIN" lookup --refresh --type A no-such-host-doh-lookup-e2e.example 2>&1); C=$?
[ $C -eq 1 ] && OK=1 || OK=0
report "$OK" "NXDOMAIN exits 1" "exit=$C; $OUT"

# 6. invalid input -> exit 2
OUT=$("$BIN" lookup "bad host" 2>&1); C=$?
[ $C -eq 2 ] && OK=1 || OK=0
report "$OK" "invalid input exits 2" "exit=$C; $OUT"

# 7. JSON output carries provenance
OUT=$("$BIN" lookup --refresh --type A --json example.com 2>&1); C=$?
if [ $C -eq 0 ] && echo "$OUT" | grep -q '"provider": "cloudflare"' \
   && echo "$OUT" | grep -q '"endpoint"'; then OK=1; else OK=0; fi
report "$OK" "JSON output includes provider + endpoint" "$OUT"

# 8. bulk via stdin -> two results
OUT=$(printf 'example.com\ncloudflare.com\n' | "$BIN" lookup --refresh --type A 2>&1); C=$?
N=$(echo "$OUT" | grep -c "via cloudflare")
[ $C -eq 0 ] && [ "$N" -eq 2 ] && OK=1 || OK=0
report "$OK" "bulk stdin returns two results" "$OUT"

# 9. MCP stdio round-trip
REQ='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"lookup","arguments":{"query":"one.one.one.one","types":["A"]}}}'
OUT=$(printf '%s\n' "$REQ" | "$BIN" mcp 2>/dev/null)
if echo "$OUT" | grep -q '"name":"doh-lookup"' && echo "$OUT" | grep -q '\\"provider\\": \\"cloudflare\\"'; then OK=1; else OK=0; fi
report "$OK" "MCP stdio initialize + lookup round-trip" "$OUT"

echo "[e2e] $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
