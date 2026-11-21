package store

// Class reads (migration 034). Every query here reads other learners' rows on the elevated
// client, so callers must have passed assertClassOwner first. A class is created by CreateGroup
// with IsClass set, so members can always be told which of their groups is one.

import "context"

// IsClass reports whether a group is a class.
func (r *Groups) IsClass(ctx context.Context, groupID string) (bool, error) {
	found, err := colOf[bool](ctx, r.q, "classes.isClass", `select is_class from study_groups where id = $1`, groupID)
	return len(found) > 0 && found[0], err
}

// OwnedClass is a class the learner teaches.
type OwnedClass struct {
	ID          string `db:"id" json:"id"`
	Name        string `db:"name" json:"name"`
	InviteCode  string `db:"invite_code" json:"inviteCode"`
	MemberCount int    `db:"member_count" json:"memberCount"`
}

// MyClasses is the classes this learner owns.
func (r *Groups) MyClasses(ctx context.Context, userID string) ([]OwnedClass, error) {
	return rowsOf[OwnedClass](ctx, r.q, "classes.mine",
		`select g.id, g.name, g.invite_code,
		        (select count(*)::int from group_members c where c.group_id = g.id) as member_count
		 from group_members m join study_groups g on g.id = m.group_id
		 where m.user_id = $1 and m.role = 'owner' and g.is_class
		 order by g.created_at desc`, userID)
}

// ClassCell is one student's mastery (0–100) on one topic.
type ClassCell struct {
	UserID    string  `db:"user_id"`
	TopicID   string  `db:"topic_id"`
	TopicName string  `db:"topic_name"`
	Score     float64 `db:"score"`
}

// ClassMastery is every student's per-topic mastery, in syllabus order. curriculumID nil means
// every topic the students have a score on.
func (r *Groups) ClassMastery(ctx context.Context, userIDs []string, curriculumID *string) ([]ClassCell, error) {
	return rowsOf[ClassCell](ctx, r.q, "classes.mastery",
		`select m.user_id, m.topic_id, t.name as topic_name, m.mastery_score::float8 as score
		 from concept_mastery m join topics t on t.id = m.topic_id
		 where m.user_id = any($1::uuid[]) and ($2::uuid is null or t.curriculum_id = $2)
		 order by t.sort_order, t.name`, userIDs, curriculumID)
}

// Downstream counts, for each topic, the topics that transitively depend on it. `union` (not
// `union all`) stops on a cycle instead of looping.
func (r *Groups) Downstream(ctx context.Context, topicIDs []string) (map[string]int, error) {
	type row struct {
		TopicID    string `db:"topic_id"`
		Downstream int    `db:"downstream"`
	}
	rows, err := rowsOf[row](ctx, r.q, "classes.downstream",
		`with recursive down(root, topic_id) as (
		   select p.prerequisite_id, p.topic_id from topic_prerequisites p where p.prerequisite_id = any($1::uuid[])
		   union
		   select d.root, p.topic_id from down d join topic_prerequisites p on p.prerequisite_id = d.topic_id
		 )
		 select root as topic_id, count(distinct topic_id)::int as downstream from down group by root`, topicIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int, len(rows))
	for _, x := range rows {
		out[x.TopicID] = x.Downstream
	}
	return out, nil
}

// ClassMisconceptionRow is a misconception across the class with the attempts that showed it.
type ClassMisconceptionRow struct {
	TopicID   string   `db:"topic_id"`
	TopicName string   `db:"topic_name"`
	Label     string   `db:"label"`
	Students  int      `db:"students"`
	Evidence  []string `db:"evidence"`
}

// ClassMisconceptions is the class's most widely held misconceptions.
func (r *Groups) ClassMisconceptions(ctx context.Context, userIDs []string, curriculumID *string, limit int) ([]ClassMisconceptionRow, error) {
	return rowsOf[ClassMisconceptionRow](ctx, r.q, "classes.misconceptions",
		`select mp.topic_id, t.name as topic_name, min(mp.misconception) as label,
		        count(distinct mp.user_id)::int as students,
		        coalesce(array_agg(e::text) filter (where e is not null), '{}') as evidence
		 from mistake_patterns mp
		 join topics t on t.id = mp.topic_id
		 left join lateral unnest(mp.evidence_attempt_ids) as e on true
		 where mp.user_id = any($1::uuid[]) and ($2::uuid is null or t.curriculum_id = $2)
		 group by mp.topic_id, t.name, lower(mp.misconception)
		 order by students desc, label
		 limit $3`, userIDs, curriculumID, limit)
}

// ReasoningQuote is a student's written reasoning on an attempt judged flawed.
type ReasoningQuote struct {
	AttemptID string `db:"id"`
	Reasoning string `db:"reasoning"`
}

// FlawedReasoning is the flawed reasoning on attemptIDs, newest first, limited to userIDs so a
// stale evidence id can never pull in someone outside the class.
func (r *Groups) FlawedReasoning(ctx context.Context, attemptIDs, userIDs []string) ([]ReasoningQuote, error) {
	if len(attemptIDs) == 0 {
		return nil, nil
	}
	return rowsOf[ReasoningQuote](ctx, r.q, "classes.flawedReasoning",
		`select a.id::text as id, a.reasoning from question_attempts a
		 where a.id = any($1::uuid[]) and a.user_id = any($2::uuid[])
		   and a.reasoning_verdict = 'flawed' and coalesce(a.reasoning, '') <> ''
		 order by a.attempted_at desc`, attemptIDs, userIDs)
}

// ClassActivity is how much the class has answered, and how many answered in the last day.
type ClassActivity struct {
	Answers     int `db:"answers" json:"answers"`
	ActiveToday int `db:"active_today" json:"activeToday"`
}

func (r *Groups) ClassActivity(ctx context.Context, userIDs []string) (*ClassActivity, error) {
	return single[ClassActivity](ctx, r.q, "classes.activity",
		`select count(*)::int as answers,
		        count(distinct user_id) filter (where attempted_at > now() - interval '24 hours')::int as active_today
		 from question_attempts where user_id = any($1::uuid[])`, userIDs)
}
