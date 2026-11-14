#!/usr/bin/env bash
# Local proof: seed a scratch Postgres, back it up, restore into a second database, compare.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
export PATH=$PATH:/Applications/Docker.app/Contents/Resources/bin
export PG_CONTAINER=${PG_CONTAINER:-restore-scratch} BACKUP_PASSPHRASE=test-passphrase
tmp=$(mktemp -d); export BACKUP_TARGET=$tmp
trap 'docker rm -f "$PG_CONTAINER" >/dev/null 2>&1; rm -rf "$tmp"' EXIT
docker run -d --name "$PG_CONTAINER" -e POSTGRES_PASSWORD=x -e POSTGRES_DB=outage postgres:17-alpine >/dev/null
until docker exec "$PG_CONTAINER" pg_isready -U postgres -d outage >/dev/null 2>&1; do sleep 1; done
sleep 3 # the image's init restarts the server once
q() { docker exec -i "$PG_CONTAINER" psql -U postgres -d "$1" -tAc "$2"; }
cat "$here/../internal/app/migrations/"*.sql | docker exec -i "$PG_CONTAINER" psql -U postgres -d outage -q -v ON_ERROR_STOP=1
q outage "INSERT INTO regions(name) VALUES ('Restore-Probe')"
f=$("$here/backup.sh")
if grep -q PGDMP "$tmp/$f"; then echo "FAIL: backup is not encrypted"; exit 1; fi
"$here/restore.sh" "$tmp/$f" "$PG_CONTAINER" restored
got=$(q restored "SELECT count(*) FROM regions WHERE name='Restore-Probe'")
[[ "$got" == 1 ]] || { echo "FAIL: probe row count = '$got'"; exit 1; }
echo "PASS: encrypted backup restored with data intact"
