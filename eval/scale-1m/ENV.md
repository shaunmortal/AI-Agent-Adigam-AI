> Copied from the run's scratch folder. `$S` below was that folder; the scripts here are the same files.

# scale1m: local 1,000,000-learner environment for Adigam

Local only: no hosted Supabase, Render or Vercel, no server/.env, AI off. Nothing in the repo was modified and nothing was committed.
Dir: `$S` (called `$S` below)

## Ports

| what | port | notes |
|---|---|---|
| Postgres 18.4 cluster `$S/pg` | 5544 (localhost TCP only) | trust auth, user `postgres`, db `postgres` |
| stub auth (`$S/stubauth-bin`) | 5545 (127.0.0.1) | stands in for `GET /auth/v1/user` |
| Redis | 6390 (127.0.0.1) | own instance with no persistence. The dev API's Redis on 6379 is not shared, because the two APIs would mix rate-limit and auth-cache keys and take each other's ingestion stream jobs |
| API (`$S/adigam-api`) | 4600 | the real `server/cmd/api` binary |

The dev processes (API :4001, Vite :5174) and the other cluster on :5433 were not touched.

## Start / stop

```sh
$S/start.sh                 # starts Postgres, Redis, stub auth, then the API; prints /health/deep
POOL=40 MAX_IN_FLIGHT=512 $S/start.sh   # optional: pgx pool size (default 12) and the shed limit (default 256)
$S/stop.sh                  # stops all four; use --keep-db to leave Postgres running
$S/smoke.sh [n1 n2 n3]      # smoke test (default: one random sample learner and two random learners)
$S/timings.sh 50            # p50/p95 per endpoint over 50 distinct random learners
```

Logs are `$S/pg.log`, `$S/api.log`, `$S/stubauth.log` and `$S/redis.log`. psql: `/Library/PostgreSQL/18/bin/psql -h localhost -p 5544 -U postgres`.

The API is started with `env -i` and nothing else in its environment:
`PORT=4600 SUPABASE_URL=http://127.0.0.1:5545 SUPABASE_PUBLISHABLE_KEY=scale1m-local SUPABASE_SECRET_KEY=
DATABASE_URL=postgres://postgres@localhost:5544/postgres?sslmode=disable REDIS_URL=redis://127.0.0.1:6390/0
ANTHROPIC_API_KEY= GEMINI_API_KEY= CORS_ORIGINS=http://localhost:4600`. Its working directory is `$S`, which has no `.env`, so `config.LoadDotenv(".env")` loads nothing.
The API has only one database setting, `DATABASE_URL`. Admin queries and per-user queries (`db.AsUser`: `SET LOCAL ROLE authenticated` plus `request.jwt.claims`) share that one pool, so RLS is enforced exactly as on Supabase.

## Auth

`server/internal/auth` sends each bearer token to `$SUPABASE_URL/auth/v1/user` and caches token -> id in Redis for 1 minute. Tokens must be 20 to 4096 characters long. The stub maps
**`scale1m-learner-<n zero-padded to 7>`** (for example `scale1m-learner-0000500`), with 1 <= n <= 1,000,000, to the id `md5('scale1m-' || n)::uuid`, which is the id load.sql gave learner n. Any other token gets a 401. No API code was changed.
The source is `$S/stubauth/main.go`. It checks itself at boot against Postgres's `md5('scale1m-1')::uuid`.

## Data (after VACUUM ANALYZE)

| table | rows | heap | indexes | total |
|---|---:|---:|---:|---:|
| concept_mastery | 10,000,073 | 1662 MB | 1427 MB | 3089 MB |
| revision_schedule | 4,712,346 (1,713,234 due today or earlier) | 519 MB | 515 MB | 1034 MB |
| learning_goal_topics | 4,000,000 | 230 MB | 272 MB | 502 MB |
| auth.users | 1,000,000 | 244 MB | 30 MB | 274 MB |
| learning_goals | 1,000,000 (all active, Class 10 Physics) | 128 MB | 105 MB | 233 MB |
| profiles | 1,000,000 | 136 MB | 30 MB | 166 MB |
| learner_profiles | 1,000,000 | 112 MB | 45 MB | 156 MB |
| question_attempts | 200,000 loaded (about 200,057 after the smoke and timing runs) | 28 MB | 42 MB | 70 MB |
| topics / questions | 50 / 121 (Class 10 Physics: 4 chapters, 28 topics, 84 MCQs, plus the CS demo bank) | | | |

**Database size: 5536 MB.** The cluster directory is 7.6 GB including WAL. The disk had 7.7 GB free at the end (96% used), so watch it before adding more data.

How the data is shaped (`$S/load.sql`):
- Learner n has a profile, a learner_profile and one active goal: "Class 10 Physics", all 4 chapters in scope, exam 30 to 180 days away, 30 to 120 minutes a day. This is what `POST /goals` writes.
- concept_mastery: each learner has 6 to 14 distinct topics (10.0 on average). Scores are spread across the range (5 buckets of 20 points: 2.88M / 2.56M / 2.26M / 1.71M / 0.58M rows). They come from a per-learner skill level plus per-topic noise, and every CHECK constraint holds.
- **Sample learners** are those with n % 100 = 0 (10,000 of them). Each has 20 real `question_attempts` spread over the last 30 days, the newest within the last hour. Their concept_mastery rows are aggregated from those attempts.
- **The other 990k learners have synthetic mastery with no attempts behind it.** The first real answer on a topic recomputes that topic from the attempt log, so its total resets. For example, learner 468122 went from `before 25` to `after 29.17` with `n=1`. This is expected with this data.
- revision_schedule covers about half of the topics with 3 or more attempts. Due dates run from 7 days overdue to 14 days ahead.
- Rebuild from scratch: initdb (see below), `$S/migrate.sh`, `psql -f database/seed/class10_physics.sql` and `demo_questions.sql`, `psql -f $S/load.sql` (6 min 22 s), `psql -f $S/verify.sql` (all 10 checks returned 0), then `vacuum (analyze)` (24 s).

## Adaptations

1. **`$S/supabase_shim.sql`** runs before the migrations. It creates:
   - the roles `anon`, `authenticated`, `service_role` (bypassrls) and `authenticator` (login)
   - schema `extensions` with pgcrypto and uuid-ossp
   - schema `auth` with `auth.users` (GoTrue columns) and `auth.identities`
   - `auth.uid()`, `auth.role()` and `auth.jwt()`, which read `request.jwt.claim.*` or `request.jwt.claims` the way Supabase does
   - schema `storage` with `buckets` and `objects`
   moddatetime ships with PG 18 contrib.
2. **pgvector is not installed** in /Library/PostgreSQL/18. Migrations 029 and 030 were applied from copies in `$S/migrations/` with 3 edits:
   - the `create extension vector` line is removed
   - `extensions.vector(768)` becomes `extensions.vector`, a shim domain over `real[]` with a SQL cosine `<=>`
   - the HNSW index is dropped
   The `source_chunks.embedding` column and `hybrid_search_source_chunks` exist. They are only used when `GEMINI_API_KEY` is set, which it is not here. The other 34 migrations were copied unchanged, and all 36 applied in order.
3. **Postgres config** (appended to `$S/pg/postgresql.conf`):
   - port 5544, listen on localhost
   - `unix_socket_directories=''`, because the socket path would exceed the 103-byte limit
   - max_connections 200, shared_buffers 1GB, effective_cache_size 4GB, work_mem 16MB, maintenance_work_mem 512MB
   - max_wal_size 2GB, random_page_cost 1.1, log_min_duration_statement 500ms
   - fsync stays on
   - `wal_level=minimal` and `max_wal_senders=0`, so the bulk load skips WAL. This does not change query behaviour.
   initdb command: `initdb -D $S/pg -U postgres --auth=trust -E UTF8 --locale=C`
4. **Bulk-load shortcuts that leave the schema unchanged.** load.sql is one transaction run with `session_replication_role=replica`, so triggers and FK checks are off. That is why handle_new_user and create_learner_profile rows were written directly. The 13 non-constraint secondary indexes were dropped and then rebuilt from their own `pg_get_indexdef()`.
   Before/after snapshots (`indexes_before.txt` and `indexes_after.txt`, all 73 indexes; `triggers_*.txt`, all 17 triggers enabled) are identical. `verify.sql` checks FK integrity and returns 0 orphans on all 10 checks.
5. Seeds: the repo's `database/seed/class10_physics.sql` and `demo_questions.sql` ran unchanged. `demo_class.sql` was not run: it inserts its own auth users and other agents are currently editing it.

## API build

`cd server && go build -o $S/adigam-api ./cmd/api` at HEAD 4458329, **plus the uncommitted working-tree changes** other agents had in server/ at 22:2x on 2026-11-21. Rebuild the binary to test a different state.

## Endpoints smoke-tested (3 learners: 990400 sample, 966403, 468122, all 2xx)

| endpoint (frontend page) | cold ms | warm ms | 50 random learners p50 / p95 ms |
|---|---|---|---|
| GET /goals (Dashboard) | 3.6–92 (first call includes auth hop and pool warm-up) | 1.1–1.6 | 8.7 / 72.8 |
| GET /learner/summary (Dashboard) | 4.2–10 | 1.8–1.9 | 10.6 / 35.8 |
| GET /recommendations/next?minutes=20 (Dashboard, Practice) | 7–12 | 4.4–6.1 | 14.0 / 129.5 |
| GET /revision/due | 4.8–6.2 | 1.7–2.2 | 10.9 / 41.4 |
| GET /readiness | 2.2–3.2 | 2.3–3.3 | 5.9 / 42.0 |
| GET /learner/subjects (Practice) | 1.7–3.5 | 1.8–2.5 | 4.4 / 27.0 |
| GET /learner/activity (Dashboard) | 4.0–4.8 | 1.9–2.6 | 7.5 / 30.0 |
| GET /sources/map/{class-10-physics} (concept map) | 1.6–2.9 | 1.8–2.2 | (not in sample) |
| GET /learner/profile, GET /classes (Dashboard) | | | 2.3 / 18.1 and 1.6 / 8.8 |
| POST /attempts (grade, record, recompute mastery, move revision) | 9.5–58.7 | 5.0–9.6 | 19.4 / 76.1 |

These are sequential single requests with no load. p95 is mostly cold pages: the 5.5 GB database does not fit in shared_buffers on an 8 GB laptop.

Bodies looked right. Each learner had a goal with daysRemaining, attempt totals and 5 weakest topics. Recommendations returned a learn_new or practice action with 3 questions, and 0 to 2 topics were due for revision. Readiness bands were not_ready or building. The map returned 4 chapters and 28 topics. Answers returned 201 with mastery before and after. A bad token returned 401.

## With AI off

- `POST /curricula/resolve`, `POST /questions/generate` and `POST /ai/tutor` return **503 AI_UNAVAILABLE** in about 1–7 ms. The tutor first logs "scope check failed, answering anyway".
- `GET /recommendations/next` stays 200, but when a learner has used up the unseen questions on a topic it calls `topUp` to generate more by AI in the background. With AI off this only logs `WARN could not top up topic`. The bank has just 3 questions per topic, so this fires often. **With AI on at scale, this endpoint launches background generations.** They are throttled per learner and topic by a 15-minute Redis key, plus a per-topic claim.
- `POST /attempts` with `reasoning` stays 201 with `reasoningVerdict: null` and logs `WARN reasoning could not be checked`. Multiple-choice grading needs no AI.
- `POST /study-plan/generate` returned 201 with no AI. `GET /misconceptions` returned 200.
