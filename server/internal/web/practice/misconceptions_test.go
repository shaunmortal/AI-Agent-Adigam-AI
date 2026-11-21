package practice

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/shaunmortal/AI-tutor/server/internal/adaptive"
	"github.com/shaunmortal/AI-tutor/server/internal/store"
)

func TestPutFirstLeadsWithTargetedQuestionsAndKeepsTheCount(t *testing.T) {
	q := func(id string) adaptive.PracticeQuestion { return adaptive.PracticeQuestion{ID: id} }
	ids := func(qs []adaptive.PracticeQuestion) (out []string) {
		for _, x := range qs {
			out = append(out, x.ID)
		}
		return out
	}
	got := ids(putFirst([]adaptive.PracticeQuestion{q("t1"), q("b")}, []adaptive.PracticeQuestion{q("a"), q("b"), q("c"), q("d")}, 4))
	want := []string{"t1", "b", "a", "c"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if n := len(putFirst([]adaptive.PracticeQuestion{q("t1"), q("t2")}, nil, 1)); n != 2 {
		t.Fatalf("targeted questions were dropped to fit the count: %d", n)
	}
}

func TestTopUpRunsOncePerLearnerAndTopicPerWindow(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	if rdb.Ping(context.Background()).Err() != nil {
		t.Skip("redis not running")
	}
	ctx := context.Background()
	topic := "test-topic-" + time.Now().Format("150405.000000")
	t.Cleanup(func() { rdb.Del(ctx, topUpKey(topic, "a"), topUpKey(topic, "b")) })

	got := []bool{topUpDue(ctx, rdb, topic, "a"), topUpDue(ctx, rdb, topic, "a"), topUpDue(ctx, rdb, topic, "b")}
	if want := []bool{true, false, true}; got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("got %v, want %v: a second visit inside the window must not pay for another batch", got, want)
	}
	if ttl := rdb.TTL(ctx, topUpKey(topic, "a")).Val(); ttl <= 0 || ttl > topUpEvery {
		t.Fatalf("window ttl %v, want up to %v", ttl, topUpEvery)
	}
}

func TestTwoCleanAnswersInARowFixAMisconception(t *testing.T) {
	streak, fixed := 0, false
	for i, step := range []struct {
		clean      bool
		wantStreak int
		wantFixed  bool
	}{
		{true, 1, false},
		{false, 0, false}, // not clean: the count starts over
		{true, 1, false},
		{true, 2, true}, // two in a row: fixed
	} {
		streak, fixed = streakAfter(streak, step.clean)
		if streak != step.wantStreak || fixed != step.wantFixed {
			t.Fatalf("step %d: got streak %d fixed %v, want %d %v", i, streak, fixed, step.wantStreak, step.wantFixed)
		}
	}
}

func TestACleanAnswerIsRightWithoutFlawedReasoning(t *testing.T) {
	said := func(v string) *string { return &v }
	for _, c := range []struct {
		v    verdict
		want bool
	}{
		{verdict{IsCorrect: true}, true},
		{verdict{IsCorrect: true, ReasoningVerdict: said("sound")}, true},
		{verdict{IsCorrect: true, ReasoningVerdict: said("missing")}, true},
		{verdict{IsCorrect: true, ReasoningVerdict: said("flawed")}, false},
		{verdict{IsCorrect: false, ReasoningVerdict: said("sound")}, false},
	} {
		if got := clean(c.v); got != c.want {
			t.Errorf("clean(%+v) = %v, want %v", c.v, got, c.want)
		}
	}
}

func TestProbesAreTheWrongOptionsTags(t *testing.T) {
	q := &store.QuestionForGrading{CorrectAnswer: json.RawMessage(`[1]`),
		OptionMisconceptions: []string{" thinks a ", "a correct option, never a probe", "", "thinks d"}}
	if got := probes(q); !slices.Equal(got, []string{"thinks a", "thinks d"}) {
		t.Fatalf("probes %q", got)
	}
	if got := probes(&store.QuestionForGrading{CorrectAnswer: json.RawMessage(`4.2`)}); got != nil {
		t.Fatalf("untagged question probes %q", got)
	}
}
