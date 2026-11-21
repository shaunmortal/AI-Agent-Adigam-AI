#!/bin/sh
# Applies supabase_shim.sql, then every repo migration in order to the scale1m cluster. The two
# pgvector migrations come from ./migrations instead: pgvector is not installed locally, so they
# use the shim's vector stand-in.
set -e
S="$(cd "$(dirname "$0")" && pwd)"
REPO_MIGRATIONS="$S/../../database/migrations"
P="/Library/PostgreSQL/18/bin/psql -h localhost -p 5544 -U postgres -d postgres -q -v ON_ERROR_STOP=1"
$P -f "$S/supabase_shim.sql"
for f in "$REPO_MIGRATIONS"/*.sql; do
  b=$(basename "$f")
  [ -f "$S/migrations/$b" ] && f="$S/migrations/$b"
  $P -f "$f" >/dev/null || { echo "FAIL $b"; exit 1; }
  echo "ok $b"
done
