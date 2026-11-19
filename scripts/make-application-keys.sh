#!/usr/bin/env bash
# Creates the key pair that protects careers applications sent from empowertours.xyz.
#  - PRIVATE key: ~/.empowertours/site/applications-private-key.pem (only you; never upload or share it)
#  - PUBLIC key:  public/apply-key.json (published on the site; it can only lock, not unlock)
# Losing the private key means applications can never be opened. Back it up.
set -euo pipefail
cd "$(dirname "$0")/.."
KEYDIR="$HOME/.empowertours/site"; PRIV="$KEYDIR/applications-private-key.pem"
if [ -e "$PRIV" ]; then echo "A private key already exists at $PRIV. Not overwriting it."; exit 1; fi
mkdir -p "$KEYDIR"; chmod 700 "$HOME/.empowertours" "$KEYDIR"
umask 077
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:4096 -out "$PRIV" 2>/dev/null
chmod 600 "$PRIV"
SPKI=$(openssl pkey -in "$PRIV" -pubout -outform DER | base64 -w0)
FP=$(openssl pkey -in "$PRIV" -pubout -outform DER | sha256sum | cut -c1-16)
printf '{ "alg": "RSA-OAEP-256+A256GCM", "kid": "%s", "spki": "%s" }\n' "$FP" "$SPKI" > public/apply-key.json
echo "Key pair created. Fingerprint: $FP"
echo "Private key saved to $PRIV (readable only by you). Back it up to your password manager."
