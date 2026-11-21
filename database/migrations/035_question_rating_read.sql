-- 035_question_rating_read.sql
--
-- Practice now picks questions by their measured difficulty (033's rating, once enough answers
-- have moved it) and serves the ones a learner should get right about 70% of the time. That
-- query runs as the learner, so the learner role needs to read the two rating columns.
--
-- A rating is how hard learners in general found a question: no answer, no explanation, nothing
-- about any one learner. Writing stays with the API's elevated client.

grant select (rating, rating_attempts) on public.questions to authenticated;
