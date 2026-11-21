-- 036_misconception_resolution.sql
--
-- A misconception can be fixed. Until now a mistake_patterns row only grew, so a learner was
-- targeted with the same belief forever. Now, each time they answer a question built to tempt a
-- misconception they hold (one of its wrong options is tagged with it), a clean answer (right,
-- and the reasoning not judged flawed) counts towards fixing it; two in a row resolve it. A wrong
-- answer on such a question, or the misconception shown again anywhere, re-opens it.
--
-- Additive only. 026 grants `authenticated` select on the whole table (no column grants), so
-- learners read the new columns under the existing RLS policy; writes stay backend-only.

alter table public.mistake_patterns
    add column if not exists resolved_at timestamptz null,
    add column if not exists clean_streak integer not null default 0;

comment on column public.mistake_patterns.resolved_at is
    'When the learner stopped showing this misconception (clean answers in a row on questions that tempt it); null while they still hold it.';
comment on column public.mistake_patterns.clean_streak is
    'Clean answers in a row on questions tempting this misconception; reset to 0 when it shows again.';
