-- Bulk-loads 1,000,000 Class 10 Physics learners into the scale1m cluster in one transaction.
-- Learner n (1..1e6) has id md5('scale1m-' || n)::uuid, which is what stub_auth.go maps the
-- token "scale1m-learner-<n>" to. Learners with n % 100 = 0 (10,000) are the "sample": they get
-- ~20 real question_attempts each and their concept_mastery is aggregated from those attempts.
-- Everyone else gets synthetic concept_mastery (6..14 topics, varied scores).
--
-- Triggers and FK checks are off for the load (session_replication_role = replica); ids are
-- generated consistently and checked afterwards in verify.sql. Secondary indexes are dropped
-- before the load and recreated from their exact pg_get_indexdef() at the end.
\set ON_ERROR_STOP on
\timing on
begin;
set local session_replication_role = replica;
set local work_mem = '256MB';
set local maintenance_work_mem = '1GB';
set local max_parallel_maintenance_workers = 4;

-- Empty tables nobody else references: truncating in this transaction lets wal_level=minimal
-- skip WAL for their data.
truncate public.concept_mastery, public.revision_schedule, public.learner_profiles, public.learning_goal_topics;

create temp table saved_idx on commit drop as
select i.indexrelid::regclass::text as name, pg_get_indexdef(i.indexrelid) as def
from pg_index i
where i.indrelid in ('auth.users'::regclass, 'public.profiles'::regclass, 'public.learner_profiles'::regclass,
                     'public.learning_goals'::regclass, 'public.learning_goal_topics'::regclass,
                     'public.concept_mastery'::regclass, 'public.revision_schedule'::regclass,
                     'public.question_attempts'::regclass)
  and not exists (select 1 from pg_constraint c where c.conindid = i.indexrelid);
select name from saved_idx order by 1;
do $$ declare r record; begin
  for r in select name from saved_idx loop execute 'drop index ' || r.name; end loop;
end $$;

-- Class 10 Physics: 28 leaf topics (i = 0..27) and their 3 questions each (qi 0 easy, 1 medium, 2 hard).
create temp table t28 on commit drop as
select t.id, t.parent_id, (row_number() over (order by p.sort_order, t.sort_order) - 1)::int as i
from topics t join topics p on p.id = t.parent_id
where t.curriculum_id = '0153f4d4-9f3d-4abe-8de0-34a72486ae55';
create temp table q84 on commit drop as
select q.id, q.topic_id, q.difficulty, q.correct_answer,
       (row_number() over (partition by q.topic_id order by array_position(array['easy','medium','hard'], q.difficulty), q.id) - 1)::int as qi
from questions q join t28 on t28.id = q.topic_id where q.is_verified;
do $$ begin
  if (select count(*) from t28) <> 28 or (select count(*) from q84) <> 84 then
    raise exception 'Class 10 Physics seed missing: run database/seed/class10_physics.sql first';
  end if;
end $$;

create temp table l on commit drop as
select n,
       md5('scale1m-' || n)::uuid as id,
       (6 + abs(hashint4extended(n, 1)) % 9)::int as k,                       -- topics touched: 6..14
       (abs(hashint4extended(n, 4)) % 28)::int as start,
       ('{1,3,5,9,11,13,15,17,19,23,25,27}'::int[])[1 + n % 12] as step,     -- coprime with 28
       (abs(hashint4extended(n, 2)) % 1000) / 1000.0 as skill,
       n % 100 = 0 as sample
from generate_series(1, 1000000) n
order by id;

insert into auth.users (instance_id, id, aud, role, email, email_confirmed_at, raw_app_meta_data,
                        raw_user_meta_data, created_at, updated_at, confirmation_token, email_change,
                        email_change_token_new, recovery_token)
select '00000000-0000-0000-0000-000000000000', id, 'authenticated', 'authenticated',
       'learner' || n || '@scale.test', now() - (n % 180) * interval '1 day',
       '{"provider":"email","providers":["email"]}', jsonb_build_object('full_name', 'Learner ' || n),
       now() - (n % 180 + 1) * interval '1 day', now(), '', '', '', ''
from l;

-- What handle_new_user would have written, plus a language mix.
insert into public.profiles (id, email, display_name, timezone, settings, created_at, updated_at, preferred_language)
select id, 'learner' || n || '@scale.test', 'Learner ' || n, 'Asia/Kolkata', '{}'::jsonb,
       now() - (n % 180 + 1) * interval '1 day', now() - (n % 180 + 1) * interval '1 day',
       (array['english','english','hindi','hinglish'])[1 + n % 4]
from l;

-- One active Class 10 Physics goal each, scoped to all four chapters (what POST /goals writes).
insert into public.learning_goals (id, user_id, title, exam_date, daily_minutes, status, created_at, updated_at, curriculum_id)
select md5('scale1m-goal-' || n)::uuid, id, 'Class 10 Physics', current_date + 30 + n % 150,
       30 + (n % 7) * 15, 'active', now() - (n % 180) * interval '1 day', now() - (n % 180) * interval '1 day',
       '0153f4d4-9f3d-4abe-8de0-34a72486ae55'
from l;
insert into public.learning_goal_topics (goal_id, topic_id)
select md5('scale1m-goal-' || l.n)::uuid, s.id
from l cross join (select distinct parent_id as id from t28) s;

-- ~20 attempts per sample learner, oldest first, the last within the past hour.
insert into public.question_attempts (id, user_id, question_id, topic_id, is_correct, given_answer,
                                      time_taken_seconds, source, attempt_number, attempted_at)
select gen_random_uuid(), a.user_id, a.question_id, a.topic_id, a.is_correct,
       case when a.is_correct then a.correct_answer
            else jsonb_build_array(((a.correct_answer ->> 0)::int + 1 + a.r % 3) % 4) end,
       10 + a.r % 110, 'practice',
       row_number() over (partition by a.user_id, a.question_id order by a.attempted_at),
       a.attempted_at
from (
  select l.id as user_id, q.id as question_id, q.topic_id, q.correct_answer,
         abs(hashint8extended(l.n::bigint * 64 + j, 5)) as r,
         (abs(hashint8extended(l.n::bigint * 64 + j, 6)) % 1000) / 1000.0
           < 0.25 + 0.6 * l.skill - 0.1 * q.qi + 0.1 as is_correct,
         now() - (19 - j) * interval '36 hours' - (abs(hashint8extended(l.n::bigint * 64 + j, 7)) % 3600) * interval '1 second'
           as attempted_at
  from l
  cross join generate_series(0, 19) j
  join t28 on t28.i = (l.start + (j % l.k) * l.step) % 28
  join q84 q on q.topic_id = t28.id and q.qi = (j / l.k + l.n) % 3
  where l.sample
) a;

-- Synthetic mastery for the 990,000 non-sample learners.
insert into public.concept_mastery (user_id, topic_id, mastery_score, total_attempts, correct_attempts,
  recent_attempts, recent_correct, easy_attempts, easy_correct, medium_attempts, medium_correct,
  hard_attempts, hard_correct, last_attempt_at, last_correct_at, correct_streak, computed_at,
  computed_version, updated_at)
select user_id, topic_id,
       least(100, round((p * 100 * total / (total + 4.0))::numeric, 2)),
       total, correct, recent, least(round(recent * p)::int, correct),
       easy, least(easy, correct),
       medium, least(medium, correct - least(easy, correct)),
       total - easy - medium, correct - least(easy, correct) - least(medium, correct - least(easy, correct)),
       last_at, case when correct > 0 then last_at - (r % 3) * interval '1 day' end,
       least(correct, (r / 7 % 6)::int), last_at, 1, last_at
from (
  select user_id, topic_id, r, p, total, correct, least(total, 10) as recent,
         (total * 3 / 10) as easy, total - (total * 3 / 10) - (total * 2 / 10) as medium, last_at
  from (
    select user_id, topic_id, r, p, total, floor(total * p)::int as correct, last_at
    from (
      select l.id as user_id, t28.id as topic_id, r,
             greatest(0.05, least(0.98, l.skill + ((r / 40 % 41) - 20) / 100.0)) as p,
             (1 + r % 40)::int as total,
             now() - (r / 1000 % 86400) * interval '1 minute' as last_at
      from (select l.*, j, abs(hashint8extended(l.n::bigint * 64 + j, 3)) as r
            from l cross join lateral generate_series(0, l.k - 1) j
            where not l.sample) l
      join t28 on t28.i = (l.start + l.j * l.step) % 28
    ) x
  ) y
) z;

-- Sample learners: mastery aggregated from their attempt log.
insert into public.concept_mastery (user_id, topic_id, mastery_score, total_attempts, correct_attempts,
  recent_attempts, recent_correct, easy_attempts, easy_correct, medium_attempts, medium_correct,
  hard_attempts, hard_correct, last_attempt_at, last_correct_at, correct_streak, computed_at,
  computed_version, updated_at)
select user_id, topic_id,
       least(100, round((100.0 * count(*) filter (where is_correct) / count(*) * count(*) / (count(*) + 4.0))::numeric, 2)),
       count(*), count(*) filter (where is_correct),
       least(count(*), 10), count(*) filter (where is_correct and rn <= 10),
       count(*) filter (where difficulty = 'easy'), count(*) filter (where difficulty = 'easy' and is_correct),
       count(*) filter (where difficulty = 'medium'), count(*) filter (where difficulty = 'medium' and is_correct),
       count(*) filter (where difficulty = 'hard'), count(*) filter (where difficulty = 'hard' and is_correct),
       max(attempted_at), max(attempted_at) filter (where is_correct),
       coalesce(min(rn) filter (where not is_correct) - 1, count(*)),
       max(attempted_at), 1, max(attempted_at)
from (
  select a.user_id, a.topic_id, a.is_correct, a.attempted_at, q.difficulty,
         row_number() over (partition by a.user_id, a.topic_id order by a.attempted_at desc) as rn
  from question_attempts a join questions q on q.id = a.question_id
) s
group by user_id, topic_id;

-- Spaced-repetition rows for about half the practised topics; due_on spans a week overdue to
-- two weeks ahead, so roughly a third are due today.
insert into public.revision_schedule (user_id, topic_id, due_on, interval_days, ease_factor, review_count,
  lapses, last_reviewed_on, last_review_accuracy, created_at, updated_at)
select user_id, topic_id, due_on, interval_days, ease_factor, review_count, least(review_count, r / 1000000 % 3),
       case when review_count > 0 then due_on - interval_days end,
       case when review_count > 0 then accuracy end,
       now() - interval '30 days', now() - (r % 30) * interval '1 day'
from (
  select user_id, topic_id, accuracy, r,
         current_date + (r % 22)::int - 7 as due_on,
         (1 + r / 100 % 30)::int as interval_days,
         round(1.30 + (r / 10000 % 171) / 100.0, 2) as ease_factor,
         (r / 100000 % 8)::int as review_count
  from (select user_id, topic_id, accuracy,
               abs(hashtextextended(user_id::text || topic_id::text, 9)) as r
        from concept_mastery where total_attempts >= 3) c
  where r % 2 = 0
) v;

-- learner_profiles (what create_learner_profile would have created), filled from mastery.
insert into public.learner_profiles (user_id, total_attempts, correct_attempts, avg_seconds_per_question,
  consistency_score, current_streak_days, longest_streak_days, last_activity_date, computed_at, computed_version, updated_at)
select m.user_id, m.total, m.correct, 20 + abs(hashtext(m.user_id::text)) % 70,
       abs(hashtext(m.user_id::text)) % 101, abs(hashtext(m.user_id::text)) % 8,
       abs(hashtext(m.user_id::text)) % 8 + abs(hashtext(m.user_id::text)) % 5, m.last_at::date, now(), 1, now()
from (select user_id, sum(total_attempts)::int as total, sum(correct_attempts)::int as correct,
             max(last_attempt_at) as last_at
      from concept_mastery group by user_id) m;

do $$ declare r record; begin
  for r in select def from saved_idx loop execute r.def; end loop;
end $$;
commit;
