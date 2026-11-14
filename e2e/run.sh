#!/usr/bin/env bash
# Browser end-to-end test of the main path against the composed stack. No live network for the app:
# registry and Outage come from e2e/fixture and e2e/seed.sql, siteverify is stubbed (e2e/compose.e2e.yaml).
# Usage: e2e/run.sh   (env: PROJECT, FRONTEND_PORT, MAILPIT_PORT)
set -euo pipefail
cd "$(dirname "$0")/.."
export PROJECT=${PROJECT:-e2e} FRONTEND_PORT=${FRONTEND_PORT:-18081} MAILPIT_PORT=${MAILPIT_PORT:-18025}
export E2E_COMPOSE="docker compose -p $PROJECT -f $PWD/compose.yaml -f $PWD/e2e/compose.e2e.yaml"
trap '$E2E_COMPOSE down -v --remove-orphans' EXIT

$E2E_COMPOSE up -d --build --wait frontend mailpit worker siteverify
$E2E_COMPOSE run --rm -v "$PWD/e2e/fixture:/fixture:ro" app import-osm /fixture/varna.osm.json
$E2E_COMPOSE exec -T postgres psql -v ON_ERROR_STOP=1 -U postgres -d outage < e2e/seed.sql

cd e2e
npm ci
npx playwright install chromium
npx playwright test
