#!/usr/bin/env bash
# Security checks for empowertours.xyz and its API endpoints. Every check is a real request.
#   scripts/security-check.sh                                   # live: site + api.empowertours.xyz
#   SITE=http://localhost:18080 API=http://localhost:18081 scripts/security-check.sh   # local
#   SITE=https://earvingallardo.com SKIP_API=1 scripts/security-check.sh               # any site, headers + exposure only
# Exits non-zero if any check fails. Safe on production: it never stores a row
# (form probes are rejected by validation or use the honeypot field).
set -uo pipefail
SITE="${SITE:-https://empowertours.xyz}"
API="${API:-https://api.empowertours.xyz}"
FINAL_SITE=$(curl -s -o /dev/null -L -w '%{url_effective}' "$SITE/" | sed 's#/$##')
pass=0 fail=0
ok() { printf '  PASS  %s\n' "$1"; pass=$((pass + 1)); }
no() { printf '  FAIL  %s  (%s)\n' "$1" "$2"; fail=$((fail + 1)); }
check() { if [ "$2" = "$3" ]; then ok "$1"; else no "$1" "got '$2', want '$3'"; fi; }
has() { if printf '%s' "$2" | grep -qiF -- "$3"; then ok "$1"; else no "$1" "missing '$3'"; fi; }
lacks() { if printf '%s' "$2" | grep -qiF -- "$3"; then no "$1" "found '$3'"; else ok "$1"; fi; }
code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }

echo "Site: $FINAL_SITE"
H=$(curl -sI "$FINAL_SITE/")
CSP=$(printf '%s' "$H" | grep -i '^content-security-policy:' | tr -d '\r')
has "CSP present" "$CSP" "default-src 'self'"
SCRIPT_SRC=$(printf '%s' "$CSP" | grep -oiE "script-src[^;]*")
lacks "CSP scripts: no unsafe-inline" "$SCRIPT_SRC" "unsafe-inline"
lacks "CSP has no unsafe-eval" "$CSP" "unsafe-eval"
has "CSP blocks framing (clickjacking)" "$CSP" "frame-ancestors 'none'"
has "CSP blocks <base> hijack" "$CSP" "base-uri 'none'"
has "CSP blocks plugins" "$CSP" "object-src 'none'"
has "HSTS" "$H" "strict-transport-security: max-age=31536000"
has "nosniff" "$H" "x-content-type-options: nosniff"
has "X-Frame-Options DENY" "$H" "x-frame-options: DENY"
has "Referrer-Policy" "$H" "referrer-policy: strict-origin-when-cross-origin"
has "Permissions-Policy denies camera/mic/geo" "$H" "camera=(), microphone=(), geolocation=()"
has "COOP same-origin" "$H" "cross-origin-opener-policy: same-origin"
lacks "No server banner" "$(printf '%s' "$H" | grep -i '^server:')" "caddy"
check "Unknown path is a real 404" "$(code "$FINAL_SITE/does-not-exist")" "404"
for p in /.git/config /.env /package.json /Caddyfile /Dockerfile /src/main.js /scripts/make-application-keys.sh; do
  check "Not exposed: $p" "$(code "$FINAL_SITE$p")" "404"
done
check "No source maps shipped" "$(curl -s "$FINAL_SITE/" | grep -c 'sourceMappingURL')" "0"
if [ "$(code "$FINAL_SITE/apply-key.json")" = 200 ]; then
  KEYJSON=$(curl -s "$FINAL_SITE/apply-key.json")
  has "Hiring key is a public SPKI" "$KEYJSON" '"spki"'
  lacks "Hiring key file holds no private key" "$KEYJSON" "PRIVATE"
fi
check "security.txt published" "$(code "$FINAL_SITE/.well-known/security.txt")" "200"
check "Privacy notice published" "$(code "$FINAL_SITE/privacy")" "200"
check "Quote start page published" "$(code "$FINAL_SITE/start")" "200"
case "$FINAL_SITE" in https://*)
  check "Plain HTTP is not served" "$(curl -s -o /dev/null -w '%{http_code}' "http://${FINAL_SITE#https://}/")" "301"
  if echo | timeout 10 openssl s_client -connect "${FINAL_SITE#https://}:443" -servername "${FINAL_SITE#https://}" -tls1_1 >/dev/null 2>&1; then no "TLS 1.1 refused" "handshake succeeded"; else ok "TLS 1.1 refused"; fi
  ;;
esac

if [ "${SKIP_API:-0}" = 1 ]; then echo; echo "$pass passed, $fail failed"; [ "$fail" -eq 0 ]; exit; fi
echo "API: $API"
J='content-type: application/json'
ACAO=$(curl -s -D - -o /dev/null -H 'Origin: https://evil.example' "$API/api/v1/site/github" | grep -i '^access-control-allow-origin' | tr -d '\r')
lacks "CORS ignores foreign origins" "$ACAO" "evil.example"
ACAO=$(curl -s -D - -o /dev/null -H 'Origin: https://empowertours.xyz' "$API/api/v1/site/github" | grep -i '^access-control-allow-origin' | tr -d '\r')
has "CORS allows empowertours.xyz" "$ACAO" "https://empowertours.xyz"
check "Feed responds" "$(code "$API/api/v1/site/github")" "200"
check "Inquiry rejects non-JSON" "$(code -X POST -d 'name=x' "$API/api/v1/site/inquiry")" "400"
check "Inquiry rejects bad email" "$(code -X POST -H "$J" -d '{"name":"x","email":"nope","message":"x"}' "$API/api/v1/site/inquiry")" "400"
BIG=$(head -c 40000 /dev/zero | tr '\0' 'a')
check "Inquiry rejects oversized body" "$(code -X POST -H "$J" -d "{\"name\":\"x\",\"email\":\"a@b.co\",\"message\":\"$BIG\"" "$API/api/v1/site/inquiry")" "400"
check "Application rejects plaintext" "$(code -X POST -H "$J" -d '{"v":1,"name":"plain text"}' "$API/api/v1/site/application")" "400"
check "Quote request rejects a bad spec" "$(code -X POST -H "$J" -d '{"spec":{"v":1,"name":"x","login":"bogus"},"name":"x","email":"a@b.co","country":"MX"}' "$API/api/v1/site/quote-request")" "400"
check "Quote review page needs a valid token" "$(code "$API/api/v1/site/admin/quotes/1?t=wrong-token")" "404"
check "Pricing admin needs a token" "$(code "$API/api/v1/site/admin/pricing")" "401"
G=$(code "$API/api/v1/site/inquiry"); if [ "$G" = 200 ]; then no "Inquiry GET refused" "got 200"; else ok "Inquiry GET refused ($G)"; fi
check "Admin read needs a token" "$(code "$API/api/v1/site/admin/inquiries")" "401"
check "Admin read refuses a wrong token" "$(code -H 'Authorization: Bearer wrong-token-wrong-token-wrong-token-1' "$API/api/v1/site/admin/applications")" "401"
# Rate limit: honeypot submissions are accepted silently and never stored, so this probe is safe live.
last=""
for i in $(seq 1 8); do last=$(code -X POST -H "$J" -d '{"name":"probe","email":"probe@example.com","message":"probe","website":"bot"}' "$API/api/v1/site/inquiry"); done
check "Form rate limit kicks in" "$last" "429"

echo
echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
