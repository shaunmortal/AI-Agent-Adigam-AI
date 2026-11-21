package learner

import (
	"math"
	"testing"
)

func TestRatingMovesTowardsWhatLearnersActuallyDo(t *testing.T) {
	ability := Ability(ptr(70.0))
	missed := UpdateRating(0, 0, ability, false)
	hit := UpdateRating(0, 0, ability, true)
	if missed <= 0 || hit >= 0 {
		t.Fatalf("a miss should make it harder and a hit easier: miss %v, hit %v", missed, hit)
	}
	// The first answer moves the rating more than the hundredth.
	if math.Abs(UpdateRating(0, 100, ability, false)) >= math.Abs(missed) {
		t.Fatal("a well-measured question should move less")
	}
}

func TestExpectedSuccessIsHalfAtEqualAbilityAndRises(t *testing.T) {
	if got := ExpectedSuccess(0.5, 0.5); math.Abs(got-0.5) > 1e-9 {
		t.Fatalf("equal ability and rating: got %v, want 0.5", got)
	}
	if ExpectedSuccess(Ability(ptr(90.0)), BaseRating("medium")) <= ExpectedSuccess(Ability(ptr(30.0)), BaseRating("medium")) {
		t.Fatal("a stronger learner should be likelier to get the same question right")
	}
	if ExpectedSuccess(0, BaseRating("hard")) >= ExpectedSuccess(0, BaseRating("easy")) {
		t.Fatal("a hard question should be less likely than an easy one")
	}
}

func TestAbilityStaysFiniteAtTheEdges(t *testing.T) {
	for _, s := range []*float64{nil, ptr(0.0), ptr(100.0)} {
		if a := Ability(s); math.IsInf(a, 0) || math.IsNaN(a) {
			t.Fatalf("ability for %v is not finite: %v", s, a)
		}
	}
}

func TestTargetRatingIsWhereTheLearnerSucceedsSeventyPercent(t *testing.T) {
	for _, m := range []*float64{nil, ptr(20.0), ptr(55.0), ptr(90.0)} {
		got := ExpectedSuccess(Ability(m), TargetRating(m))
		if math.Abs(got-TargetSuccess) > 1e-9 {
			t.Fatalf("mastery %v: expected success at the target is %v, want %v", m, got, TargetSuccess)
		}
	}
	if TargetRating(ptr(80.0)) <= TargetRating(ptr(30.0)) {
		t.Fatal("a stronger learner should be targeted at harder questions")
	}
}
