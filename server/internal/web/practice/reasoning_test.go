package practice

import (
	"strings"
	"testing"
)

func ptr(s string) *string { return &s }

func check(verdict string, label *string) *reasoningCheck {
	return &reasoningCheck{Verdict: verdict, Misconception: label, Feedback: "because"}
}

func TestWithReasoningLeavesAnUnjudgedAnswerAsItWas(t *testing.T) {
	v := verdict{IsCorrect: false, Misconception: ptr("option tag")}
	got := withReasoning(v, nil)
	if got.Misconception == nil || *got.Misconception != "option tag" || got.ReasoningVerdict != nil || got.ReasoningFeedback != nil {
		t.Fatalf("no reasoning must change nothing, got %+v", got)
	}
}

func TestWithReasoningPrefersTheIdeaInTheReasoning(t *testing.T) {
	got := withReasoning(verdict{IsCorrect: false, Misconception: ptr("option tag")}, check("flawed", ptr("current gets used up")))
	if got.Misconception == nil || *got.Misconception != "current gets used up" {
		t.Fatalf("reasoning label should win, got %v", got.Misconception)
	}
}

func TestWithReasoningTreatsASoundWrongAnswerAsASlip(t *testing.T) {
	got := withReasoning(verdict{IsCorrect: false, Misconception: ptr("option tag")}, check("sound", nil))
	if got.Misconception != nil {
		t.Fatalf("a slip shows no misconception, got %v", *got.Misconception)
	}
}

func TestWithReasoningCatchesARightAnswerForAWrongReason(t *testing.T) {
	got := withReasoning(verdict{IsCorrect: true}, check("flawed", ptr("thinner wire carries more current")))
	if got.Misconception == nil || *got.ReasoningVerdict != "flawed" {
		t.Fatalf("right answer, flawed reasoning should record the idea, got %+v", got)
	}
}

func TestWithReasoningKeepsTheOptionTagWhenReasoningIsVague(t *testing.T) {
	got := withReasoning(verdict{IsCorrect: false, Misconception: ptr("option tag")}, check("missing", nil))
	if got.Misconception == nil || *got.Misconception != "option tag" {
		t.Fatalf("vague reasoning should fall back to the option tag, got %v", got.Misconception)
	}
}

func TestReasoningFieldCleansAndBounds(t *testing.T) {
	if s, err := reasoningField("  why\x00 not  "); err != nil || s == nil || *s != "why not" {
		t.Fatalf("want trimmed and NUL-free, got %v %v", s, err)
	}
	if s, err := reasoningField("   "); err != nil || s != nil {
		t.Fatalf("blank means none, got %v %v", s, err)
	}
	if _, err := reasoningField(strings.Repeat("a", maxReasoning+1)); err == nil {
		t.Fatal("over-long reasoning should be rejected")
	}
	if _, err := reasoningField(7); err == nil {
		t.Fatal("a non-string should be rejected")
	}
}
