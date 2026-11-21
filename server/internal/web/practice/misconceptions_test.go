package practice

import (
	"testing"

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
