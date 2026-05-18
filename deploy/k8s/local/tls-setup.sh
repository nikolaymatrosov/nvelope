#!/usr/bin/env sh
# Issues a locally-trusted TLS certificate for nvelope.local with mkcert and
# stores it as the `nvelope-tls` secret the gateway mounts. The certificate is
# machine-specific (signed by your mkcert CA), so it is created here rather
# than committed. Re-run any time; it is idempotent.
#
#   sh deploy/k8s/local/tls-setup.sh
#
set -e

command -v mkcert >/dev/null 2>&1 || { echo "mkcert not found — install it first (brew install mkcert)"; exit 1; }

DIR=$(mktemp -d)
trap 'rm -rf "$DIR"' EXIT

mkcert -cert-file "$DIR/tls.crt" -key-file "$DIR/tls.key" nvelope.local

kubectl create namespace nvelope --dry-run=client -o yaml | kubectl apply -f -
kubectl -n nvelope create secret tls nvelope-tls \
  --cert="$DIR/tls.crt" --key="$DIR/tls.key" \
  --dry-run=client -o yaml | kubectl apply -f -

echo "nvelope-tls secret updated for nvelope.local"
