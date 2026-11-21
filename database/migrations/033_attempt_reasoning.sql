-- 033_attempt_reasoning.sql
--
-- Two additions to the learner model:
--   - the student's one-line reasoning for an answer, and the AI's verdict on it. A wrong idea
--     behind an answer is found in the reasoning, not only in which option was picked; a right
--     answer given for a wrong reason is caught too.
--   - a measured difficulty per question. difficulty stays the author's label; rating starts
--     from it and is moved Elo-style by real answers (null until the first answer).

alter table public.question_attempts add column if not exists reasoning text;
alter table public.question_attempts add column if not exists reasoning_verdict text;

alter table public.question_attempts drop constraint if exists question_attempts_reasoning_verdict_check;
alter table public.question_attempts add constraint question_attempts_reasoning_verdict_check
    check (reasoning_verdict in ('sound', 'flawed', 'missing'));

alter table public.question_attempts drop constraint if exists question_attempts_reasoning_length;
alter table public.question_attempts add constraint question_attempts_reasoning_length
    check (reasoning is null or char_length(reasoning) <= 600);

alter table public.questions add column if not exists rating real;
alter table public.questions add column if not exists rating_attempts int not null default 0;

-- No new grants: question_attempts is already granted table-wide to its owner (005), so a
-- learner reads their own reasoning back; rating and rating_attempts stay server side.
