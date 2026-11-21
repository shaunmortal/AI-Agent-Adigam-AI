-- Integrity checks for load.sql (FK checks were off during the load). Every count must be 0.
select 'profiles without auth user' as check, count(*) from profiles p where not exists (select 1 from auth.users u where u.id = p.id)
union all select 'auth users without profile', count(*) from auth.users u where not exists (select 1 from profiles p where p.id = u.id)
union all select 'profiles without learner_profile', count(*) from profiles p where not exists (select 1 from learner_profiles lp where lp.user_id = p.id)
union all select 'profiles without active goal', count(*) from profiles p where not exists (select 1 from learning_goals g where g.user_id = p.id and g.status = 'active')
union all select 'goal_topics bad goal', count(*) from learning_goal_topics gt where not exists (select 1 from learning_goals g where g.id = gt.goal_id)
union all select 'mastery bad user', count(*) from concept_mastery m where not exists (select 1 from profiles p where p.id = m.user_id)
union all select 'mastery bad topic', count(*) from concept_mastery m where not exists (select 1 from topics t where t.id = m.topic_id)
union all select 'revision without mastery', count(*) from revision_schedule r where not exists (select 1 from concept_mastery m where m.user_id = r.user_id and m.topic_id = r.topic_id)
union all select 'attempts bad user', count(*) from question_attempts a where not exists (select 1 from profiles p where p.id = a.user_id)
union all select 'attempts topic mismatch', count(*) from question_attempts a join questions q on q.id = a.question_id where q.topic_id <> a.topic_id;
