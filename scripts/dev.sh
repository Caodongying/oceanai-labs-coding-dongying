#!/usr/bin/env sh
set -eu
cd "$(dirname "$0")/.."
if [ -f frontend/package.json ]; then
  docker compose --profile ui up --build -d
  echo "UI: http://localhost:3000  API: http://localhost:8080/api/v1"
else
  docker compose up --build -d
  echo "Backend ready at http://localhost:8080/api/v1 (frontend not present yet)"
fi
