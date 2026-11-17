#!/usr/bin/env sh
set -eu
cd "$(dirname "$0")/.."
docker compose build backend
docker compose run --rm --no-deps backend seed-large "$@"
