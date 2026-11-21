package learner

import "math"

// Measured question difficulty. An author labels a question easy, medium or hard; real answers
// then move a rating, Elo-style, towards how hard learners actually find it.
const (
	minAbilityP = 0.02
	maxAbilityP = 0.98

	// Early answers move a rating most; it settles as evidence builds up.
	ratingK    = 0.6
	minRatingK = 0.15
)

// BaseRating is where an uncalibrated question starts, from its author's label.
func BaseRating(difficulty string) float64 {
	switch difficulty {
	case "easy":
		return -1
	case "hard":
		return 1
	}
	return 0
}

// Ability puts a 0-100 mastery score on the rating scale; nil is the neutral prior.
func Ability(masteryScore *float64) float64 {
	p := neutralPrior
	if masteryScore != nil {
		p = *masteryScore / 100
	}
	p = clamp(p, minAbilityP, maxAbilityP)
	return math.Log(p / (1 - p))
}

// ExpectedSuccess is the chance a learner at ability answers a question at rating correctly.
func ExpectedSuccess(ability, rating float64) float64 {
	return 1 / (1 + math.Exp(-(ability - rating)))
}

// UpdateRating moves a question's rating after one answer: a miss where a hit was expected
// makes the question harder, a hit where a miss was expected makes it easier.
func UpdateRating(rating float64, attempts int, ability float64, correct bool) float64 {
	k := max(minRatingK, ratingK/math.Sqrt(float64(attempts)+1))
	actual := 0.0
	if correct {
		actual = 1
	}
	return rating + k*(ExpectedSuccess(ability, rating)-actual)
}
