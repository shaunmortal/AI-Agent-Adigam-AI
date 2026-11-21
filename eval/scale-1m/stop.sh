#!/bin/sh
# Stops what start.sh started. Pass --keep-db to leave Postgres running.
S="$(cd "$(dirname "$0")" && pwd)"
for p in api stubauth; do [ -f $S/$p.pid ] && kill $(cat $S/$p.pid) 2>/dev/null; rm -f $S/$p.pid; done
redis-cli -p 6390 shutdown nosave 2>/dev/null
[ "$1" = "--keep-db" ] || /Library/PostgreSQL/18/bin/pg_ctl -D $S/pg -w stop -m fast
