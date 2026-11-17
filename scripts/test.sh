#!/usr/bin/env sh
set -eu
cd "$(dirname "$0")/.."
docker compose build backend
if [ -f frontend/package.json ]; then
  docker compose --profile ui run --rm --no-deps frontend sh -c 'npm install && npm run build'
fi
echo "Build verification complete. Automated tests are deferred for this MVP."
