#!/usr/bin/env bash
# restore.sh FILE.dump.enc CONTAINER DB: decrypt and restore into a scratch database (never prod).
# For an s3 object, fetch it first: aws s3 cp s3://... FILE --endpoint-url ...
set -euo pipefail
: "${BACKUP_PASSPHRASE:?}"
file=$1 container=$2 db=$3
[[ "$container" != outage-radar-postgres ]] || { echo "refusing to restore into prod container" >&2; exit 1; }
docker exec "$container" createdb -U postgres "$db"
openssl enc -d -aes-256-cbc -pbkdf2 -pass env:BACKUP_PASSPHRASE -in "$file" |
  docker exec -i "$container" pg_restore -U postgres -d "$db" --no-owner
