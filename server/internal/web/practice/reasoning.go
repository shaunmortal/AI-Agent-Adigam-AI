package practice

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/shaunmortal/AI-tutor/server/internal/ai"
	"github.com/shaunmortal/AI-tutor/server/internal/app"
	"github.com/shaunmortal/AI-tutor/server/internal/assessment"
	"github.com/shaunmortal/AI-tutor/server/internal/ratelimit"
	"github.com/shaunmortal/AI-tutor/server/internal/store"
)

// A learner can say why they chose an option. The model reads that line for the idea behind the
// answer, so a misconception is found in the reasoning rather than guessed from the option alone,
// and a right answer given for a wrong reason is caught too.

// reasoningCheck is the model's judgement of the line (ai.ReasoningCheckSchema).
type reasoningCheck struct {
	Verdict       string  `json:"verdict"`
	Misconception *string `json:"misconception"`
	Evidence      string  `json:"evidence"`
	Feedback      string  `json:"feedback"`
}

const (
	minReasoning     = 3
	maxLabel         = 120
	reasoningTimeout = 20 * time.Second
	// How many of the learner's own misconceptions on the topic the model is shown to reuse.
	knownPerTopic = 10
	// checksPerHour caps the paid call on an otherwise free route. Past it an answer is marked
	// without its reasoning judged, never refused.
	checksPerHour = 60
)

var verdicts = []string{"sound", "flawed", "missing"}

// reasoningChecker asks the model. A variable so tests can stand in for it.
var reasoningChecker = func(ctx context.Context, client *ai.Client, prompt string) (reasoningCheck, error) {
	if client == nil {
		return reasoningCheck{}, errAIUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, reasoningTimeout)
	defer cancel()
	var c reasoningCheck
	_, err := client.JSON(ctx, ai.Request{System: ai.ReasoningCheckSystem, Schema: ai.ReasoningCheckSchema,
		Prompt: prompt, MaxTokens: 1500, Quality: ai.Standard}, &c)
	return c, err
}

// checkReasoning judges a choice answer's reasoning. Nil when there is nothing to judge or the
// model could not judge it: the answer is then marked exactly as it was before reasoning existed.
func checkReasoning(ctx context.Context, d *app.Deps, userID string, q *store.QuestionForGrading,
	a givenAnswer, reasoning *string) *reasoningCheck {
	if reasoning == nil || utf8.RuneCountInString(*reasoning) < minReasoning || a.Options == nil {
		return nil
	}
	if q.QuestionType != string(assessment.MCQ) && q.QuestionType != string(assessment.MSQ) {
		return nil
	}
	if !ratelimit.Allow(ctx, d.Redis, "reasoning-check", userID, checksPerHour, time.Hour) {
		return nil
	}
	var correct []int
	_ = json.Unmarshal(q.CorrectAnswer, &correct) // grade has already checked its shape
	prompt := ai.ReasoningCheckPrompt(topicLabel(ctx, d, q.TopicID), q.Body, q.Options, correct, a.Options,
		*reasoning, knownMisconceptions(ctx, d, userID, q))
	c, err := reasoningChecker(ctx, d.AI, prompt)
	if err != nil {
		slog.Warn("reasoning could not be checked", "question", q.ID, "err", err)
		return nil
	}
	if !slices.Contains(verdicts, c.Verdict) {
		c.Verdict = "missing"
	}
	return &c
}

// withReasoning folds the judgement into the mark. The reasoning outranks the option's tag: a
// wrong answer reasoned soundly is a slip and shows no misconception, and a right answer
// reasoned on a wrong idea records that idea.
func withReasoning(v verdict, c *reasoningCheck) verdict {
	if c == nil {
		return v
	}
	v.ReasoningVerdict = &c.Verdict
	v.ReasoningFeedback = clip(c.Feedback, 1000)
	switch {
	case c.Verdict == "sound" && !v.IsCorrect:
		v.Misconception = nil
	case c.Verdict == "flawed" && c.Misconception != nil:
		if label := clip(*c.Misconception, maxLabel); label != nil {
			v.Misconception = label
		}
	}
	return v
}

// topicLabel is "Subject › Topic" for the prompt, or "" when the topic cannot be read.
func topicLabel(ctx context.Context, d *app.Deps, topicID string) string {
	t, err := store.NewQuestions(d.DB.Admin).FindTopicWithSubject(ctx, topicID)
	if err != nil || t == nil {
		return ""
	}
	if t.SubjectName == t.Name {
		return t.Name
	}
	return t.SubjectName + " › " + t.Name
}

// knownMisconceptions are the labels the model should reuse when one fits: the question's own
// distractor rationales, then what this learner has already shown on the topic.
func knownMisconceptions(ctx context.Context, d *app.Deps, userID string, q *store.QuestionForGrading) []string {
	var out []string
	add := func(label string) {
		if label = strings.TrimSpace(label); label != "" && !slices.Contains(out, label) {
			out = append(out, label)
		}
	}
	for _, m := range q.OptionMisconceptions {
		add(m)
	}
	labels, err := store.NewMistakes(d.DB.Admin).Labels(ctx, userID, q.TopicID, knownPerTopic)
	if err != nil {
		slog.Warn("could not read the learner's misconceptions", "topic", q.TopicID, "err", err)
		return out
	}
	for _, l := range labels {
		add(l)
	}
	return out
}
