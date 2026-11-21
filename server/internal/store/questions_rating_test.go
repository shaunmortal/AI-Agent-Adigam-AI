package store

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/shaunmortal/AI-tutor/server/internal/adaptive"
	"github.com/shaunmortal/AI-tutor/server/internal/learner"
)

// Practice picks questions by measured difficulty once enough answers have rated them, and puts
// the ones nearest the learner's 70% target first. Runs inside the fixture's rolled-back tx.
func TestMeasuredDifficultyDecidesLevelAndOrder(t *testing.T) {
	f := setup(t)
	r := NewQuestions(f.tx)
	level := func(id string, d adaptive.Difficulty) bool {
		qs, err := r.FindForPractice(f.ctx, adaptive.PracticeQuery{TopicID: f.topicID, Difficulty: d, Only: []string{id}, Limit: 1})
		must(t, err)
		return len(qs) == 1
	}

	// Labelled easy, but learners keep missing it: a high rating.
	_, err := f.tx.Exec(f.ctx, `update questions set difficulty = 'easy', rating = 1.4, rating_attempts = $2 where id = $1`,
		f.questionID, learner.CalibratedAfter-1)
	must(t, err)
	check(t, level(f.questionID, adaptive.Easy) && !level(f.questionID, adaptive.Hard),
		"too few answers: the author's label still decides")

	_, err = f.tx.Exec(f.ctx, `update questions set rating_attempts = $2 where id = $1`, f.questionID, learner.CalibratedAfter)
	must(t, err)
	check(t, level(f.questionID, adaptive.Hard) && !level(f.questionID, adaptive.Easy),
		"measured enough: served as hard despite its easy label")

	// The learner role can run the same query (migration 035's grant).
	f.asUser(t, f.u1, func(q Q) {
		target := 1.4
		_, err := NewQuestions(q).FindForPractice(f.ctx, adaptive.PracticeQuery{TopicID: f.topicID, Limit: 5, TargetRating: &target})
		must(t, err)
	})

	var other string
	if err := f.tx.QueryRow(f.ctx, `select id::text from questions where topic_id = $1 and id <> $2 limit 1`,
		f.topicID, f.questionID).Scan(&other); err != nil {
		t.Skip("topic has one question, ordering not checkable")
	}
	_, err = f.tx.Exec(f.ctx, `update questions set rating = -2, rating_attempts = 0 where id = $1`, other)
	must(t, err)
	for target, want := range map[float64]string{1.2: f.questionID, -1.8: other} {
		qs, err := r.FindForPractice(f.ctx, adaptive.PracticeQuery{TopicID: f.topicID, Only: []string{f.questionID, other}, Limit: 2, TargetRating: &target})
		must(t, err)
		check(t, len(qs) == 2 && qs[0].ID == want, "target %v: want %s first, got %+v", target, want, qs)
	}
}

func TestFindExposingMatchesLabelsIgnoringCaseAndSkipsSeen(t *testing.T) {
	f := setup(t)
	var hasOptions bool
	must(t, f.tx.QueryRow(f.ctx, `select cardinality(options) > 1 from questions where id = $1`, f.questionID).Scan(&hasOptions))
	if !hasOptions {
		t.Skip("fixture question has no options")
	}
	_, err := f.tx.Exec(f.ctx, `update questions set is_verified = true, option_misconceptions =
		(select array_agg(case when i = 2 then '  Current gets used up ' else '' end order by i) from generate_subscripts(options, 1) i)
		where id = $1`, f.questionID)
	must(t, err)
	r := NewQuestions(f.tx)
	has := func(labels, exclude []string) (string, bool) {
		got, err := r.FindExposing(f.ctx, f.topicID, labels, exclude, 50)
		must(t, err)
		for _, e := range got {
			if e.ID == f.questionID {
				return e.Label, true
			}
		}
		return "", false
	}
	label, ok := has([]string{"current GETS used up"}, nil)
	check(t, ok && label == "Current gets used up", "case-insensitive match with its trimmed label, got %q %v", label, ok)
	_, ok = has([]string{"current gets used up"}, []string{f.questionID})
	check(t, !ok, "a question already answered is not served again")
	_, ok = has([]string{"light bends away"}, nil)
	check(t, !ok, "another misconception does not match")
}

// Two learners answering one question at once. Each attempt's foreign key holds KEY SHARE on the
// question until commit, so the rating lock taken after it must not conflict with the other
// learner's, or the two transactions wait on each other (deadlock). Both transactions roll back.
func TestRatingLockLetsTwoLearnersAnswerAtOnce(t *testing.T) {
	f := setup(t) // for the question; its own transaction is not used here
	var inserted sync.WaitGroup
	inserted.Add(2)
	errs := make([]error, 2)
	var done sync.WaitGroup
	for i := range errs {
		done.Go(func() {
			ready := sync.OnceFunc(inserted.Done)
			defer ready() // never leave the other learner waiting at the barrier
			errs[i] = func() error {
				conn, err := pgx.Connect(f.ctx, databaseURL())
				if err != nil {
					return err
				}
				defer func() { _ = conn.Close(f.ctx) }()
				tx, err := conn.Begin(f.ctx)
				if err != nil {
					return err
				}
				defer func() { _ = tx.Rollback(f.ctx) }() // always: this is a shared database
				if _, err := tx.Exec(f.ctx, `set local statement_timeout = '10s'`); err != nil {
					return err
				}
				var user string
				if err := tx.QueryRow(f.ctx, `with id as (select gen_random_uuid() as id)
					insert into auth.users (id, email) select id, id || '@adigam.test' from id returning id::text`).
					Scan(&user); err != nil {
					return err
				}
				if _, err := NewAttempts(tx).Insert(f.ctx, NewAttempt{UserID: user, QuestionID: f.questionID,
					GivenAnswer: json.RawMessage(`{"selectedOptions":[0]}`), Source: "practice"}); err != nil {
					return err
				}
				// Both attempts are in (both KEY SHARE locks held) before either locks the rating.
				ready()
				inserted.Wait()
				q := NewQuestions(tx)
				if _, err := q.FindRatingForUpdate(f.ctx, f.questionID); err != nil {
					return err
				}
				return q.SaveRating(f.ctx, f.questionID, 0)
			}()
		})
	}
	done.Wait()
	for _, err := range errs {
		must(t, err)
	}
}
