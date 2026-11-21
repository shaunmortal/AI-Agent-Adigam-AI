package store

import (
	"strings"
	"testing"
)

// A resolved misconception leaves the learner's list and the class count, still findable so a
// relapse re-opens it (036). Runs inside the fixture's rolled-back tx.
func TestResolvedMisconceptionsLeaveTopAndReopenWhenShownAgain(t *testing.T) {
	f := setup(t)
	r := NewMistakes(f.tx)
	a := f.attempt(t, f.u1, false)
	must(t, r.Record(f.ctx, f.u1, f.topicID, "Confuses mass with weight", a.ID))
	must(t, r.Record(f.ctx, f.u1, f.topicID, "thinks heavier falls faster", a.ID))
	must(t, r.Record(f.ctx, f.u2, f.topicID, "confuses mass with weight", f.attempt(t, f.u2, false).ID))

	probed := func(label string) ProbedPattern {
		t.Helper()
		held, err := r.FindProbed(f.ctx, f.u1, f.topicID, []string{label, "never held"})
		must(t, err)
		if len(held) != 1 {
			t.Fatalf("probed %q: %+v", label, held)
		}
		return held[0]
	}
	mass := probed("  CONFUSES mass with weight ")
	check(t, mass.Label == "Confuses mass with weight" && mass.CleanStreak == 0 && !mass.Resolved, "held %+v", mass)

	must(t, r.SaveStreak(f.ctx, mass.ID, 2, true))
	top, err := r.Top(f.ctx, f.u1, nil, 5, 10)
	must(t, err)
	check(t, len(top) == 1 && top[0].Label == "thinks heavier falls faster", "resolved row still listed: %+v", top)
	labels, err := r.Labels(f.ctx, f.u1, f.topicID, 10)
	must(t, err)
	check(t, len(labels) == 2, "the judge's labels leave out a fixed one: %q", labels)
	mass = probed("confuses mass with weight")
	check(t, mass.Resolved && mass.CleanStreak == 2, "resolved row not findable: %+v", mass)

	// The class: u2 still holds it, u1 fixed it.
	class := func() map[string]ClassMisconceptionRow {
		t.Helper()
		rows, err := NewGroups(f.tx).ClassMisconceptions(f.ctx, []string{f.u1, f.u2}, nil, 10)
		must(t, err)
		out := map[string]ClassMisconceptionRow{}
		for _, x := range rows {
			out[strings.ToLower(x.Label)] = x
		}
		return out
	}
	row := class()["confuses mass with weight"]
	check(t, row.Students == 1 && row.Fixed == 1, "class row %+v", row)

	// One nobody holds any more is no longer something to teach.
	falls := probed("thinks heavier falls faster")
	must(t, r.SaveStreak(f.ctx, falls.ID, 2, true))
	_, listed := class()["thinks heavier falls faster"]
	check(t, !listed, "a misconception nobody holds is still on the class list")

	// Shown again: re-opened, the streak starts over.
	must(t, r.Record(f.ctx, f.u1, f.topicID, "confuses MASS with weight", a.ID))
	mass = probed("confuses mass with weight")
	check(t, !mass.Resolved && mass.CleanStreak == 0, "not re-opened: %+v", mass)
	top, err = r.Top(f.ctx, f.u1, nil, 5, 10)
	must(t, err)
	check(t, len(top) == 1 && top[0].Label == "Confuses mass with weight", "re-opened row not listed: %+v", top)

	// Learners read the new columns of their own rows.
	f.asUser(t, f.u1, func(q Q) {
		var n int
		must(t, q.QueryRow(f.ctx, `select count(*) from mistake_patterns where resolved_at is not null or clean_streak = 0`).Scan(&n))
		check(t, n == 2, "own rows with the new columns: %d", n)
	})
}
