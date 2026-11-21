#!/bin/sh
# Starts the scale1m stack: Postgres :5544, Redis :6390, stub auth :5545, API :4600 (AI off).
S="$(cd "$(dirname "$0")" && pwd)"
cd $S || exit 1
/Library/PostgreSQL/18/bin/pg_ctl -D $S/pg -l $S/pg.log -w status >/dev/null 2>&1 || /Library/PostgreSQL/18/bin/pg_ctl -D $S/pg -l $S/pg.log -w start
redis-cli -p 6390 ping >/dev/null 2>&1 || redis-server --port 6390 --bind 127.0.0.1 --save '' --appendonly no \
  --daemonize yes --pidfile $S/redis.pid --logfile $S/redis.log --dir $S
[ -f $S/stubauth.pid ] && kill -0 $(cat $S/stubauth.pid) 2>/dev/null || { nohup $S/stubauth-bin -addr 127.0.0.1:5545 >$S/stubauth.log 2>&1 & echo $! >$S/stubauth.pid; }
# env -i: nothing from the calling shell (no AI keys, no hosted DATABASE_URL); cwd has no .env.
[ -f $S/api.pid ] && kill -0 $(cat $S/api.pid) 2>/dev/null || {
  env -i PATH=/usr/bin:/bin HOME=$HOME PORT=4600 \
    SUPABASE_URL=http://127.0.0.1:5545 SUPABASE_PUBLISHABLE_KEY=scale1m-local SUPABASE_SECRET_KEY= \
    DATABASE_URL="postgres://postgres@localhost:5544/postgres?sslmode=disable${POOL:+&pool_max_conns=$POOL}" \
    REDIS_URL=redis://127.0.0.1:6390/0 ANTHROPIC_API_KEY= GEMINI_API_KEY= CORS_ORIGINS=http://localhost:4600 \
    ${MAX_IN_FLIGHT:+MAX_IN_FLIGHT=$MAX_IN_FLIGHT} \
    nohup $S/adigam-api >$S/api.log 2>&1 & echo $! >$S/api.pid; }
for i in 1 2 3 4 5 6 7 8 9 10; do curl -sf http://localhost:4600/api/v1/health/deep >/dev/null && break; sleep 0.5; done
curl -s http://localhost:4600/api/v1/health/deep; echo
