-- 034_classes.sql
--
-- A class is a study group whose owner is the teacher.
--
-- A plain study group shares a short, deliberate list (name, questions answered, streak, topics
-- mastered — see 022), and nothing about that changes here: is_class defaults to false.
--
-- A class shares more, and only upward: its owner may see each member's per-topic mastery,
-- misconceptions and the reasoning behind wrong answers, so they can see where the whole class is
-- stuck. Members are told this when they create or join a class.
--
-- No policy is loosened. Learners still read only their own concept_mastery, mistake_patterns and
-- question_attempts. The class view is read by the API on the elevated client, and only after it
-- has checked that the caller is the owner of a group with is_class set (assertClassOwner in
-- server/internal/web/learning/class.go). A member who is not the owner gets 403.

alter table public.study_groups
    add column if not exists is_class boolean not null default false;

comment on column public.study_groups.is_class is
    'A class: the owner (teacher) may see members'' per-topic mastery, misconceptions and reasoning. Set at creation only.';
