#!/usr/bin/env sh
set -eu
cd "$(dirname "$0")/.."
api_port=${API_PORT:-8080}
if [ -f frontend/package.json ]; then
  docker compose --profile ui up --build -d
else
  docker compose up --build -d
fi
docker build --target test -f backend/Dockerfile backend
if [ -f frontend/package.json ]; then
  docker compose --profile ui run --rm --no-deps frontend sh -c 'npm install && npm run build && if node -e "process.exit(require(\"./package.json\").scripts.test ? 0 : 1)"; then CI=1 npm test; fi'
fi
base="http://localhost:$api_port/api/v1"
ready=0
for attempt in 1 2 3 4 5 6 7 8 9 10 11 12; do
  if curl -fsS "$base/demo/admins" >/dev/null; then ready=1; break; fi
  sleep 2
done
if [ "$ready" -ne 1 ]; then echo "API did not become ready" >&2; exit 1; fi
curl -fsS -H 'X-Admin-Email: alice@east.test' "$base/me" >/dev/null
curl -fsS -H 'X-Admin-Email: alice@east.test' "$base/tenants/east/certificates" >/dev/null
curl -fsS -H 'X-Admin-Email: bob@west.test' "$base/tenants/west/certificates/export" >/dev/null
code=$(curl -sS -o /dev/null -w '%{http_code}' -H 'X-Admin-Email: bob@west.test' "$base/tenants/east/certificates")
if [ "$code" != 404 ]; then echo "Cross-tenant request returned $code, expected 404" >&2; exit 1; fi
echo "Backend tests, build checks, and API smoke checks passed."
