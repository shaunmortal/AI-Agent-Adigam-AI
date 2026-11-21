package groups

// Classes: a study group with is_class set, owned by its teacher.
//
// Joining a class is consent to wider sharing than SharedProgress: the owner, and only the owner,
// sees each member's per-topic mastery, misconceptions and the reasoning behind wrong answers.
// The gate is assertClassOwner in web/learning; this file only shapes what that gate lets through.
// Member user ids never leave it: a student row carries an opaque key and a display name.

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"
)

// belowHalf is the mastery score (0–100) under which a student counts as stuck on a topic.
const belowHalf = 50

// MasteryCell is one student's score on one topic. Student indexes the class's student list.
type MasteryCell struct {
	Student   int
	TopicID   string
	TopicName string
	Score     float64
}

// Gap is a topic the class is weak on, ranked by how much re-teaching it is worth.
type Gap struct {
	TopicID      string  `json:"topicId"`
	TopicName    string  `json:"topicName"`
	AvgMastery   float64 `json:"avgMastery"`
	BelowHalf    int     `json:"belowHalf"`
	StudentsSeen int     `json:"studentsSeen"`
	Downstream   int     `json:"downstream"`
	Priority     float64 `json:"priority"`
}

// ClassStudent is one heatmap row.
type ClassStudent struct {
	Key        string             `json:"key"`
	Name       string             `json:"name"`
	AvgMastery float64            `json:"avgMastery"`
	Mastery    map[string]float64 `json:"mastery"`
}

// ClassMisconception is a wrong idea held across the class, in the students' own words.
type ClassMisconception struct {
	TopicID   string `json:"topicId"`
	TopicName string `json:"topicName"`
	Label     string `json:"label"`
	// Students hold it now; Fixed once held it and have resolved it.
	Students int      `json:"students"`
	Fixed    int      `json:"fixed"`
	Quotes   []string `json:"quotes"`
}

// ClassGaps ranks every topic at least one student has a score on: weak on average, weak for many,
// and blocking many later topics (downstream = topics that transitively depend on it) rank first.
func ClassGaps(cells []MasteryCell, downstream map[string]int) []Gap {
	type acc struct {
		name     string
		sum      float64
		n, below int
	}
	byTopic := map[string]*acc{}
	var order []string
	for _, c := range cells {
		a, ok := byTopic[c.TopicID]
		if !ok {
			a = &acc{name: c.TopicName}
			byTopic[c.TopicID] = a
			order = append(order, c.TopicID)
		}
		a.sum += c.Score
		a.n++
		if c.Score < belowHalf {
			a.below++
		}
	}
	gaps := make([]Gap, 0, len(order))
	for _, id := range order {
		a := byTopic[id]
		avg := a.sum / float64(a.n)
		down := downstream[id]
		priority := (1 - avg/100) * (0.5 + float64(a.below)/float64(a.n)) * (1 + float64(down)/5)
		gaps = append(gaps, Gap{id, a.name, round(avg, 1), a.below, a.n, down, round(priority, 3)})
	}
	slices.SortStableFunc(gaps, func(a, b Gap) int { return cmp.Compare(b.Priority, a.Priority) })
	return gaps
}

// StudentRows builds the heatmap rows, weakest student first. names[i] is student i.
func StudentRows(names []string, cells []MasteryCell) []ClassStudent {
	rows := make([]ClassStudent, len(names))
	for i, name := range names {
		rows[i] = ClassStudent{Name: name, Mastery: map[string]float64{}}
	}
	for _, c := range cells {
		rows[c.Student].Mastery[c.TopicID] = round(c.Score, 1)
	}
	for i := range rows {
		sum := 0.0
		for _, s := range rows[i].Mastery {
			sum += s
		}
		if n := len(rows[i].Mastery); n > 0 {
			rows[i].AvgMastery = round(sum/float64(n), 1)
		}
	}
	slices.SortStableFunc(rows, func(a, b ClassStudent) int {
		return cmp.Or(cmp.Compare(a.AvgMastery, b.AvgMastery), localeCompare(a.Name, b.Name))
	})
	for i := range rows {
		rows[i].Key = fmt.Sprintf("s%d", i+1)
	}
	return rows
}

const (
	summaryGaps           = 5
	summaryMisconceptions = 6
	weakestPerGap         = 4
)

// ClassSummary is the plain-text brief the re-teach plan is written from. It carries display
// names, because the plan groups students by name; the teacher already sees them.
func ClassSummary(className string, students []ClassStudent, gaps []Gap, mcs []ClassMisconception) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Class: %s, %d students.\n\nBiggest gaps, highest priority first:\n", className, len(students))
	for _, g := range gaps[:min(len(gaps), summaryGaps)] {
		var have []ClassStudent
		for _, s := range students {
			if _, ok := s.Mastery[g.TopicID]; ok {
				have = append(have, s)
			}
		}
		slices.SortStableFunc(have, func(a, b ClassStudent) int { return cmp.Compare(a.Mastery[g.TopicID], b.Mastery[g.TopicID]) })
		weakest := make([]string, 0, weakestPerGap)
		for _, s := range have[:min(len(have), weakestPerGap)] {
			weakest = append(weakest, s.Name)
		}
		fmt.Fprintf(&b, "- %s: class average %.0f%%, %d of %d students below 50%%, blocks %d later topics. Weakest: %s\n",
			g.TopicName, g.AvgMastery, g.BelowHalf, g.StudentsSeen, g.Downstream, strings.Join(weakest, ", "))
	}
	b.WriteString("\nMost common misconceptions:\n")
	for _, m := range mcs[:min(len(mcs), summaryMisconceptions)] {
		fmt.Fprintf(&b, "- \"%s\" (%s): %d students.", m.Label, m.TopicName, m.Students)
		if len(m.Quotes) > 0 {
			fmt.Fprintf(&b, " Their reasoning: \"%s\"", strings.Join(m.Quotes, "\" | \""))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func round(x float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(x*p) / p
}
