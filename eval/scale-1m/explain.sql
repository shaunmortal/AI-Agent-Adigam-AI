-- EXPLAIN (ANALYZE, BUFFERS) of the 3 statements with the most total execution time in the
-- pass3-c256 run (pg_stat_statements), each for a fresh random learner (first touch: cold rows).
-- Reads run the way db.AsUser runs them: role authenticated + request.jwt.claims, so RLS applies.
\set ON_ERROR_STOP on
select (floor(random() * 1000000) + 1)::int as n1, (floor(random() * 1000000) + 1)::int as n2,
       (floor(random() * 1000000) + 1)::int as n3 \gset
select :n1 as learner_1, :n2 as learner_2, :n3 as learner_3;
select md5('scale1m-' || :n1)::uuid as u1, md5('scale1m-' || :n2)::uuid as u2, md5('scale1m-' || :n3)::uuid as u3 \gset
-- leaf topics in learner 2's goal scope (what FindGoalSubjects passes as $2)
select array_agg(t.id)::text as leaves from learning_goals g join learning_goal_topics gt on gt.goal_id = g.id
  join topics t on t.parent_id = gt.topic_id where g.user_id = :'u2' and g.status = 'active' \gset
-- a topic learner 3 already has a mastery row for (the upsert's conflict/update path)
select topic_id as t3 from concept_mastery where user_id = :'u3' order by topic_id limit 1 \gset

\echo '== 1. topics.findMastery (GET /recommendations/next), as authenticated, learner' :n1
begin;
select set_config('request.jwt.claims', json_build_object('sub', :'u1', 'role', 'authenticated')::text, true),
       set_config('role', 'authenticated', true) \g /dev/null
explain (analyze, buffers, settings)
select topic_id, mastery_score, recent_accuracy, recent_attempts, total_attempts, last_attempt_at
  from concept_mastery where user_id = :'u1';
rollback;

\echo '== 2. learners.findGoalSubjects (GET /learner/subjects, GET /readiness), as authenticated, learner' :n2
begin;
select set_config('request.jwt.claims', json_build_object('sub', :'u2', 'role', 'authenticated')::text, true),
       set_config('role', 'authenticated', true) \g /dev/null
explain (analyze, buffers, settings)
select topic_id, mastery_score, total_attempts from concept_mastery
 where user_id = :'u2' and topic_id = any(:'leaves'::uuid[]);
rollback;

\echo '== 3. mastery upsert (POST /attempts recompute), admin client, learner' :n3 ', rolled back'
begin;
explain (analyze, buffers, settings)
insert into concept_mastery (user_id, topic_id, mastery_score, total_attempts, correct_attempts,
   recent_attempts, recent_correct, easy_attempts, easy_correct, medium_attempts, medium_correct,
   hard_attempts, hard_correct, correct_streak, last_attempt_at, last_correct_at, computed_at, computed_version)
 values (:'u3', :'t3', 55.5, 1, 1, 1, 1, 0, 0, 1, 1, 0, 0, 1, now(), now(), now(), 1)
 on conflict (user_id, topic_id) do update set
   mastery_score = excluded.mastery_score, total_attempts = excluded.total_attempts,
   correct_attempts = excluded.correct_attempts, recent_attempts = excluded.recent_attempts,
   recent_correct = excluded.recent_correct, easy_attempts = excluded.easy_attempts,
   easy_correct = excluded.easy_correct, medium_attempts = excluded.medium_attempts,
   medium_correct = excluded.medium_correct, hard_attempts = excluded.hard_attempts,
   hard_correct = excluded.hard_correct, correct_streak = excluded.correct_streak,
   last_attempt_at = excluded.last_attempt_at, last_correct_at = excluded.last_correct_at,
   computed_at = excluded.computed_at, computed_version = excluded.computed_version;
rollback;
