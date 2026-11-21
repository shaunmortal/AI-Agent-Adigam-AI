package practice

import (
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/shaunmortal/AI-tutor/server/internal/adaptive"
	"github.com/shaunmortal/AI-tutor/server/internal/app"
	"github.com/shaunmortal/AI-tutor/server/internal/store"
)

// maxTargeted is how many questions in one session aim at the learner's own misconceptions:
// enough to see whether the belief is still there, not so many that it is the whole session.
const maxTargeted = 2

// targetedGenerationEvery stops a topic whose generated questions never come back tagged with a
// learner's label from paying for a new batch on every visit.
const targetedGenerationEvery = 6 * time.Hour

// aimAtMisconceptions moves questions whose wrong options expose this learner's own
// misconceptions on the topic to the front of the session, and says which misconception each one
// is aimed at (question id -> label). When the bank has none they have not seen, it asks in the
// background for a batch written to expose those beliefs (generating reports that it did), and
// the session goes ahead as it was.
func aimAtMisconceptions(ctx context.Context, d *app.Deps, userID, topicID, difficulty string,
	sel *adaptive.SelectedQuestions, count int) (aimed map[string]string, generating bool, err error) {
	var labels, attempted []string
	if err := d.DB.AsUser(ctx, userID, func(tx pgx.Tx) error {
		rows, err := store.NewMistakes(tx).Top(ctx, userID, []string{topicID}, maxTargeted, maxTargeted)
		if err != nil || len(rows) == 0 {
			return err
		}
		for _, r := range rows {
			labels = append(labels, r.Label)
		}
		attempted, err = store.NewQuestions(tx).FindAttemptedIDs(ctx, userID, topicID)
		return err
	}); err != nil || len(labels) == 0 {
		return nil, false, err
	}

	// Elevated: which option a misconception leads to is part of the answer key.
	found, err := store.NewQuestions(d.DB.Admin).FindExposing(ctx, topicID, labels, attempted, maxTargeted)
	if err != nil {
		return nil, false, err
	}
	if len(found) == 0 {
		// Only when nothing else is writing this topic, so the batch is not skipped for a lock
		// while the once-per-window guard is already spent.
		if d.AI != nil && d.Redis.Exists(ctx, "qgen:"+topicID).Val() == 0 &&
			d.Redis.SetNX(ctx, "qgen-mc:"+topicID+":"+userID, "1", targetedGenerationEvery).Val() {
			topUp(d, genInput{topicID, difficulty, firstBatch, userID, labels})
			return nil, true, nil
		}
		return nil, false, nil
	}

	ids := make([]string, len(found))
	for i, f := range found {
		ids[i] = f.ID
	}
	var lead []adaptive.PracticeQuestion
	if err := d.DB.AsUser(ctx, userID, func(tx pgx.Tx) error {
		lead, err = store.NewQuestions(tx).FindForPractice(ctx, adaptive.PracticeQuery{TopicID: topicID, Only: ids, Limit: len(ids)})
		return err
	}); err != nil || len(lead) == 0 {
		return nil, false, err
	}

	sel.Questions = putFirst(lead, sel.Questions, count)
	aimed = map[string]string{}
	for _, f := range found {
		if slices.ContainsFunc(lead, func(q adaptive.PracticeQuestion) bool { return q.ID == f.ID }) {
			aimed[f.ID] = f.Label
		}
	}
	return aimed, false, nil
}

// putFirst is lead followed by rest, without repeats, at most n long (never shorter than lead).
func putFirst(lead, rest []adaptive.PracticeQuestion, n int) []adaptive.PracticeQuestion {
	out := slices.Clone(lead)
	for _, q := range rest {
		if len(out) >= max(n, len(lead)) {
			break
		}
		if !slices.ContainsFunc(out, func(x adaptive.PracticeQuestion) bool { return x.ID == q.ID }) {
			out = append(out, q)
		}
	}
	return out
}
