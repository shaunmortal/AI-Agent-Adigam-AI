// Command evalreasoning measures the judge that reads a student's reasoning on a multiple-choice
// answer, against a hand-written test set (eval/reasoning-testset.json).
//
// Each item is judged through practice.JudgeReasoning, the same call the practice route makes:
// the question as the database holds it, the same prompt, schema, known labels, model tier and
// timeout. The learner is a fresh one, so the known labels are the question's own option labels.
// Every call goes to the real model.
//
// Usage, from server/ (reads server/.env):
//
//	go run ./cmd/evalreasoning -in ../eval/reasoning-testset.json
//
// Writes reasoning-eval.json (one entry per item) and reasoning-eval-summary.json to -out.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/shaunmortal/AI-tutor/server/internal/ai"
	"github.com/shaunmortal/AI-tutor/server/internal/app"
	"github.com/shaunmortal/AI-tutor/server/internal/assessment"
	"github.com/shaunmortal/AI-tutor/server/internal/config"
	"github.com/shaunmortal/AI-tutor/server/internal/db"
	"github.com/shaunmortal/AI-tutor/server/internal/store"
	"github.com/shaunmortal/AI-tutor/server/internal/web/practice"
)

// freshLearner has no recorded misconceptions, so only the question's own labels are known.
const freshLearner = "00000000-0000-0000-0000-000000000000"

// soundCategories are where a "flawed" verdict is a false alarm.
var soundCategories = []string{"right-option-sound", "wrong-option-sound-slip"}

type testItem struct {
	ID                    string   `json:"id"`
	Category              string   `json:"category"`
	Language              string   `json:"language"`
	QuestionID            string   `json:"questionId"`
	TopicName             string   `json:"topicName"`
	Question              string   `json:"question"`
	Options               []string `json:"options"`
	ChosenIndex           int      `json:"chosenIndex"`
	Reasoning             string   `json:"reasoning"`
	ExpectedVerdict       string   `json:"expectedVerdict"`
	ExpectedMisconception *string  `json:"expectedMisconception"`
}

type result struct {
	ID                    string   `json:"id"`
	Category              string   `json:"category"`
	Language              string   `json:"language"`
	Reasoning             string   `json:"reasoning"`
	KnownLabels           []string `json:"knownLabels"` // the question's option labels, as sent
	ExpectedVerdict       string   `json:"expectedVerdict"`
	GotVerdict            string   `json:"gotVerdict"`
	ExpectedMisconception *string  `json:"expectedMisconception"`
	GotMisconception      *string  `json:"gotMisconception"`
	Evidence              string   `json:"evidence"`
	Feedback              string   `json:"feedback"`
	VerdictCorrect        bool     `json:"verdictCorrect"`
	// MisconceptionExact is set only for items expected flawed.
	MisconceptionExact *bool  `json:"misconceptionExact,omitempty"`
	FalseAlarm         bool   `json:"falseAlarm"`
	Attempts           int    `json:"attempts"`
	Error              string `json:"error,omitempty"`
}

type tally struct {
	Items          int `json:"items"`
	VerdictCorrect int `json:"verdictCorrect"`
	// Over the items expected flawed.
	ExpectedFlawed     int `json:"expectedFlawed"`
	MisconceptionExact int `json:"misconceptionExact"`
}

type summary struct {
	Testset            string            `json:"testset"`
	Items              int               `json:"items"`
	Failed             []string          `json:"failed"`
	Overall            tally             `json:"overall"`
	ByCategory         map[string]tally  `json:"byCategory"`
	ByLanguage         map[string]tally  `json:"byLanguage"`
	SoundOrSlipItems   int               `json:"soundOrSlipItems"`
	FalseAlarms        int               `json:"falseAlarms"`
	FalseAlarmIDs      []string          `json:"falseAlarmIds"`
	VerdictMissIDs     []string          `json:"verdictMissIds"`
	LabelMismatches    []labelMismatch   `json:"labelMismatches"` // to judge by hand for a lenient count
	QuestionMismatches map[string]string `json:"questionMismatches,omitempty"`
}

type labelMismatch struct {
	ID       string  `json:"id"`
	Expected string  `json:"expected"`
	Got      *string `json:"got"`
}

func main() {
	in := flag.String("in", "../eval/reasoning-testset.json", "test set to run")
	out := flag.String("out", "../eval/results", "directory for reasoning-eval.json and reasoning-eval-summary.json")
	flag.Parse()
	if err := run(*in, *out); err != nil {
		log.Fatal(err)
	}
}

func run(inPath, outDir string) error {
	items, err := readTestset(inPath)
	if err != nil {
		return err
	}
	config.LoadDotenv(".env")
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx := context.Background()
	database, err := db.Open(ctx, cfg.DatabaseURL, cfg.DBTransactionPooler)
	if err != nil {
		return err
	}
	defer database.Close()
	client := ai.New(cfg.AISettings())
	if client == nil {
		return errors.New("no API key for AI_PROVIDER " + cfg.AIProvider)
	}
	d := &app.Deps{Cfg: cfg, DB: database, AI: client}
	questions := store.NewAttempts(database.Admin)

	results := make([]result, 0, len(items))
	mismatches := map[string]string{}
	for _, it := range items {
		q, err := questions.FindForGrading(ctx, it.QuestionID)
		if err != nil {
			return fmt.Errorf("%s: question %s: %w", it.ID, it.QuestionID, err)
		}
		if why := differs(it, q); why != "" {
			mismatches[it.ID] = why
		}
		r := judge(ctx, d, it, q)
		fmt.Printf("%-4s %-30s expected %-7s got %-7s %v\n", it.ID, it.Category, it.ExpectedVerdict, r.GotVerdict, deref(r.GotMisconception))
		results = append(results, r)
	}

	s := summarize(inPath, results)
	if len(mismatches) > 0 {
		s.QuestionMismatches = mismatches
	}
	if err := writeJSON(filepath.Join(outDir, "reasoning-eval.json"), results); err != nil {
		return err
	}
	return writeJSON(filepath.Join(outDir, "reasoning-eval-summary.json"), s)
}

// judge asks the model, retrying once when the call fails.
func judge(ctx context.Context, d *app.Deps, it testItem, q *store.QuestionForGrading) result {
	r := result{ID: it.ID, Category: it.Category, Language: it.Language, Reasoning: it.Reasoning,
		KnownLabels: nonEmpty(q.OptionMisconceptions), ExpectedVerdict: it.ExpectedVerdict,
		ExpectedMisconception: it.ExpectedMisconception}
	var err error
	for r.Attempts = 1; r.Attempts <= 2; r.Attempts++ {
		c, e := practice.JudgeReasoning(ctx, d, freshLearner, q, []int{it.ChosenIndex}, it.Reasoning)
		if err = e; err == nil {
			r.GotVerdict, r.GotMisconception, r.Evidence, r.Feedback = c.Verdict, c.Misconception, c.Evidence, c.Feedback
			break
		}
	}
	if err != nil {
		r.Attempts, r.Error = 2, err.Error()
		return r
	}
	r.VerdictCorrect = r.GotVerdict == it.ExpectedVerdict
	if it.ExpectedVerdict == "flawed" {
		// The app keeps the judge's label only on a flawed verdict (withReasoning), so only then can it match.
		exact := r.GotVerdict == "flawed" && it.ExpectedMisconception != nil && r.GotMisconception != nil &&
			normLabel(*r.GotMisconception) == normLabel(*it.ExpectedMisconception)
		r.MisconceptionExact = &exact
	}
	r.FalseAlarm = slices.Contains(soundCategories, it.Category) && r.GotVerdict == "flawed"
	return r
}

func summarize(inPath string, results []result) summary {
	s := summary{Testset: inPath, Items: len(results), Failed: []string{}, ByCategory: map[string]tally{},
		ByLanguage: map[string]tally{}, FalseAlarmIDs: []string{}, VerdictMissIDs: []string{}, LabelMismatches: []labelMismatch{}}
	for _, r := range results {
		if r.Error != "" {
			s.Failed = append(s.Failed, r.ID)
			continue
		}
		s.Overall = s.Overall.add(r)
		s.ByCategory[r.Category] = s.ByCategory[r.Category].add(r)
		s.ByLanguage[r.Language] = s.ByLanguage[r.Language].add(r)
		if !r.VerdictCorrect {
			s.VerdictMissIDs = append(s.VerdictMissIDs, r.ID)
		}
		if slices.Contains(soundCategories, r.Category) {
			s.SoundOrSlipItems++
			if r.FalseAlarm {
				s.FalseAlarms++
				s.FalseAlarmIDs = append(s.FalseAlarmIDs, r.ID)
			}
		}
		if r.MisconceptionExact != nil && !*r.MisconceptionExact {
			s.LabelMismatches = append(s.LabelMismatches, labelMismatch{ID: r.ID, Expected: deref(r.ExpectedMisconception), Got: r.GotMisconception})
		}
	}
	return s
}

func (t tally) add(r result) tally {
	t.Items++
	if r.VerdictCorrect {
		t.VerdictCorrect++
	}
	if r.MisconceptionExact != nil {
		t.ExpectedFlawed++
		if *r.MisconceptionExact {
			t.MisconceptionExact++
		}
	}
	return t
}

func readTestset(path string) ([]testItem, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var set struct {
		Items []testItem `json:"items"`
	}
	if err := json.Unmarshal(raw, &set); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(set.Items) == 0 {
		return nil, fmt.Errorf("%s: no items", path)
	}
	for _, it := range set.Items {
		switch {
		case it.ID == "" || it.QuestionID == "" || strings.TrimSpace(it.Reasoning) == "":
			return nil, fmt.Errorf("item %q: id, questionId and reasoning are required", it.ID)
		case !slices.Contains([]string{"sound", "flawed", "missing"}, it.ExpectedVerdict):
			return nil, fmt.Errorf("item %s: expectedVerdict %q", it.ID, it.ExpectedVerdict)
		case it.ExpectedVerdict == "flawed" && it.ExpectedMisconception == nil:
			return nil, fmt.Errorf("item %s: flawed with no expectedMisconception", it.ID)
		case it.ChosenIndex < 0 || it.ChosenIndex >= len(it.Options):
			return nil, fmt.Errorf("item %s: chosenIndex %d out of range", it.ID, it.ChosenIndex)
		}
	}
	return set.Items, nil
}

// differs says how the test set's copy of a question differs from the database's ("" if not).
func differs(it testItem, q *store.QuestionForGrading) string {
	switch {
	case q.QuestionType != string(assessment.MCQ) && q.QuestionType != string(assessment.MSQ):
		return "question type " + q.QuestionType
	case q.Body != it.Question:
		return "question text"
	case !slices.Equal(q.Options, it.Options):
		return "options"
	}
	return ""
}

// normLabel compares labels ignoring case and spacing.
func normLabel(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

func nonEmpty(labels []string) []string {
	out := []string{}
	for _, l := range labels {
		if l = strings.TrimSpace(l); l != "" && !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return "<null>"
	}
	return *s
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
