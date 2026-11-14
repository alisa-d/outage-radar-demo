#!/usr/bin/env bash
# Encrypted pg_dump -> BACKUP_TARGET (s3://bucket/prefix, or a local directory for tests).
# Env: BACKUP_PASSPHRASE, BACKUP_TARGET, PG_CONTAINER (default outage-radar-postgres),
# S3_ENDPOINT + AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY when the target is s3://.
set -euo pipefail
: "${BACKUP_PASSPHRASE:?}" "${BACKUP_TARGET:?}"
name="outage-$(date -u +%Y%m%dT%H%M%SZ).dump.enc"
dump() {
  docker exec "${PG_CONTAINER:-outage-radar-postgres}" pg_dump -U postgres -Fc outage |
    openssl enc -aes-256-cbc -pbkdf2 -salt -pass env:BACKUP_PASSPHRASE
}
if [[ "$BACKUP_TARGET" == s3://* ]]; then
  dump | aws s3 cp - "$BACKUP_TARGET/$name" ${S3_ENDPOINT:+--endpoint-url "$S3_ENDPOINT"}
else
  dump > "$BACKUP_TARGET/$name.part" && mv "$BACKUP_TARGET/$name.part" "$BACKUP_TARGET/$name"
fi
echo "$name"
