#!/bin/sh
# Build the Pine binary, scaffold a throwaway repo, and serve it for e2e tests.
set -e
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
make build >/dev/null
TMP="$(mktemp -d)"
cd "$TMP"
git init -q
git -c user.email=e2e@pine.test -c user.name=e2e commit -q --allow-empty -m init
"$ROOT/pine" init >/dev/null
"$ROOT/pine" create --type feature --title "Seed feature" >/dev/null
exec "$ROOT/pine" serve --port 3413
