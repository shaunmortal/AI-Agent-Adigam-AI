package learning

// Classes (migration 034): a study group whose owner is the teacher. A class lets its owner see
// what a study group never shows — each member's per-topic mastery, misconceptions and the
// reasoning behind wrong answers. Every read runs on Admin (other learners' rows), so
// assertClassOwner is the whole gate and runs before anything is fetched.

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/shaunmortal/AI-tutor/server/internal/ai"
	"github.com/shaunmortal/AI-tutor/server/internal/auth"
	"github.com/shaunmortal/AI-tutor/server/internal/groups"
	"github.com/shaunmortal/AI-tutor/server/internal/httpx"
	"github.com/shaunmortal/AI-tutor/server/internal/learner"
	"github.com/shaunmortal/AI-tutor/server/internal/store"
)

const (
	classMisconceptionLimit = 10
	quotesPerMisconception  = 3
	planCacheTTL            = 2 * time.Minute
)

var errNotClassOwner = httpx.E(http.StatusForbidden, "NOT_CLASS_OWNER",
	"Only the teacher who created this class can see its insights.")

// assertClassOwner returns the class and its students (every member but the owner). A group you
// are not in is the same 404 as assertMember, so ids are not confirmed; a member who is not the
// owner, or a group that is not a class, is 403.
func assertClassOwner(ctx context.Context, g *store.Groups, userID, groupID string) (*store.GroupRow, []string, error) {
	group, err := assertMember(ctx, g, userID, groupID)
	if err != nil {
		return nil, nil, err
	}
	isClass, err := g.IsClass(ctx, groupID)
	if err != nil {
		return nil, nil, err
	}
	members, err := g.FindMemberIDs(ctx, groupID)
	if err != nil {
		return nil, nil, err
	}
	owner := false
	var students []string
	for _, m := range members {
		if m.UserID == userID {
			owner = m.Role == "owner"
			continue
		}
		students = append(students, m.UserID)
	}
	if !isClass || !owner {
		return nil, nil, errNotClassOwner
	}
	return group, students, nil
}

type topicRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type classInsights struct {
	Totals struct {
		Students    int `json:"students"`
		Answers     int `json:"answers"`
		ActiveToday int `json:"activeToday"`
	} `json:"totals"`
	Gaps           []groups.Gap                `json:"gaps"`
	Misconceptions []groups.ClassMisconception `json:"misconceptions"`
	Topics         []topicRef                  `json:"topics"`
	Students       []groups.ClassStudent       `json:"students"`
}

func buildInsights(ctx context.Context, g *store.Groups, class *store.GroupRow, studentIDs []string) (*classInsights, error) {
	out := &classInsights{Gaps: []groups.Gap{}, Misconceptions: []groups.ClassMisconception{}, Topics: []topicRef{}, Students: []groups.ClassStudent{}}
	out.Totals.Students = len(studentIDs)
	if len(studentIDs) == 0 {
		return out, nil
	}

	progress, err := g.FindSharedProgress(ctx, studentIDs, learner.MasteryBar)
	if err != nil {
		return nil, err
	}
	index := make(map[string]int, len(studentIDs))
	names := make([]string, len(studentIDs))
	for i, id := range studentIDs {
		index[id] = i
		names[i] = "Learner"
		if p, ok := progress[id]; ok {
			names[i] = p.DisplayName
		}
	}

	rows, err := g.ClassMastery(ctx, studentIDs, class.CurriculumID)
	if err != nil {
		return nil, err
	}
	cells := make([]groups.MasteryCell, len(rows))
	seen := map[string]bool{}
	var topicIDs []string
	for i, r := range rows {
		cells[i] = groups.MasteryCell{Student: index[r.UserID], TopicID: r.TopicID, TopicName: r.TopicName, Score: r.Score}
		if !seen[r.TopicID] {
			seen[r.TopicID] = true
			topicIDs = append(topicIDs, r.TopicID)
			out.Topics = append(out.Topics, topicRef{r.TopicID, r.TopicName})
		}
	}
	downstream, err := g.Downstream(ctx, topicIDs)
	if err != nil {
		return nil, err
	}
	out.Gaps = groups.ClassGaps(cells, downstream)
	out.Students = groups.StudentRows(names, cells)

	if out.Misconceptions, err = classMisconceptions(ctx, g, studentIDs, class.CurriculumID); err != nil {
		return nil, err
	}
	activity, err := g.ClassActivity(ctx, studentIDs)
	if err != nil {
		return nil, err
	}
	out.Totals.Answers, out.Totals.ActiveToday = activity.Answers, activity.ActiveToday
	return out, nil
}

// classMisconceptions attaches, to each misconception, up to three of the students' own flawed
// reasoning lines from the attempts that showed it.
func classMisconceptions(ctx context.Context, g *store.Groups, studentIDs []string, curriculumID *string) ([]groups.ClassMisconception, error) {
	rows, err := g.ClassMisconceptions(ctx, studentIDs, curriculumID, classMisconceptionLimit)
	if err != nil {
		return nil, err
	}
	var evidence []string
	for _, r := range rows {
		evidence = append(evidence, r.Evidence...)
	}
	quotes, err := g.FlawedReasoning(ctx, evidence, studentIDs)
	if err != nil {
		return nil, err
	}
	out := make([]groups.ClassMisconception, len(rows))
	for i, r := range rows {
		mine := map[string]bool{}
		for _, id := range r.Evidence {
			mine[id] = true
		}
		said := map[string]bool{}
		picked := []string{}
		for _, q := range quotes {
			if mine[q.AttemptID] && !said[q.Reasoning] && len(picked) < quotesPerMisconception {
				said[q.Reasoning] = true
				picked = append(picked, q.Reasoning)
			}
		}
		out[i] = groups.ClassMisconception{TopicID: r.TopicID, TopicName: r.TopicName, Label: r.Label, Students: r.Students, Fixed: r.Fixed, Quotes: picked}
	}
	return out, nil
}

// GET /classes — the classes this learner teaches, for the link to each dashboard.
func (h handlers) listMyClasses(w http.ResponseWriter, r *http.Request) error {
	classes, err := store.NewGroups(h.d.DB.Admin).MyClasses(r.Context(), auth.UserID(r))
	if err != nil {
		return err
	}
	if classes == nil {
		classes = []store.OwnedClass{}
	}
	httpx.OK(w, http.StatusOK, map[string]any{"classes": classes})
	return nil
}

// GET /groups/{id}/insights — the teacher's view of the class. Owner of a class only.
func (h handlers) getClassInsights(w http.ResponseWriter, r *http.Request) error {
	id, err := groupID(r)
	if err != nil {
		return err
	}
	ctx, g := r.Context(), store.NewGroups(h.d.DB.Admin)
	class, students, err := assertClassOwner(ctx, g, auth.UserID(r), id)
	if err != nil {
		return err
	}
	insights, err := buildInsights(ctx, g, class, students)
	if err != nil {
		return err
	}
	httpx.OK(w, http.StatusOK, map[string]any{
		"class":    map[string]any{"id": class.ID, "name": class.Name, "inviteCode": class.InviteCode},
		"insights": insights,
	})
	return nil
}

type reteachPlan struct {
	Headline string `json:"headline"`
	Steps    []struct {
		Topic     string `json:"topic"`
		Minutes   int    `json:"minutes"`
		Activity  string `json:"activity"`
		Addresses string `json:"addresses"`
	} `json:"steps"`
	Groups []struct {
		Title    string   `json:"title"`
		Students []string `json:"students"`
		Focus    string   `json:"focus"`
	} `json:"groups"`
}

// studentsOnce keeps each student in the first group the model put them in, so a teacher never
// gets one student sent to two tables; a group left empty is dropped.
func studentsOnce(plan reteachPlan) reteachPlan {
	out := plan
	out.Groups = plan.Groups[:0:0]
	seen := map[string]bool{}
	for _, grp := range plan.Groups {
		kept := []string{}
		for _, s := range grp.Students {
			if k := strings.ToLower(strings.TrimSpace(s)); k != "" && !seen[k] {
				seen[k] = true
				kept = append(kept, s)
			}
		}
		if len(kept) > 0 {
			grp.Students = kept
			out.Groups = append(out.Groups, grp)
		}
	}
	return out
}

// POST /groups/{id}/insights/plan — a re-teach plan from the class's gaps. Cached per class for
// two minutes, so a teacher pressing the button twice does not pay for two plans.
func (h handlers) generateClassPlan(w http.ResponseWriter, r *http.Request) error {
	id, err := groupID(r)
	if err != nil {
		return err
	}
	ctx, g := r.Context(), store.NewGroups(h.d.DB.Admin)
	class, students, err := assertClassOwner(ctx, g, auth.UserID(r), id)
	if err != nil {
		return err
	}

	key := "class-plan:" + class.ID
	if cached, err := h.d.Redis.Get(ctx, key).Bytes(); err == nil {
		var plan reteachPlan
		if json.Unmarshal(cached, &plan) == nil {
			httpx.OK(w, http.StatusOK, map[string]any{"plan": plan})
			return nil
		}
	}

	insights, err := buildInsights(ctx, g, class, students)
	if err != nil {
		return err
	}
	if len(insights.Gaps) == 0 {
		return httpx.E(http.StatusConflict, "NO_CLASS_DATA",
			"No answers from this class yet. Ask your students to take the diagnostic first.")
	}
	var plan reteachPlan
	summary := groups.ClassSummary(class.Name, insights.Students, insights.Gaps, insights.Misconceptions)
	if _, err := h.d.AI.JSON(ctx, ai.Request{System: ai.ReteachPlanSystem, Prompt: ai.ReteachPlanPrompt(summary),
		Schema: ai.ReteachPlanSchema, Quality: ai.Standard, MaxTokens: 4000}, &plan); err != nil {
		return err
	}
	plan = studentsOnce(plan)
	if b, err := json.Marshal(plan); err == nil {
		if err := h.d.Redis.Set(ctx, key, b, planCacheTTL).Err(); err != nil {
			slog.Warn("class plan cache unavailable", "err", err)
		}
	}
	httpx.OK(w, http.StatusOK, map[string]any{"plan": plan})
	return nil
}
