package practice

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/shaunmortal/AI-tutor/server/internal/adaptive"
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
