package groups

import (
	"strings"
	"testing"
)

func TestClassGapsRanksBlockingWeakTopicsFirst(t *testing.T) {
	cells := []MasteryCell{
		{0, "current", "Current", 30}, {1, "current", "Current", 40},
		{0, "optics", "Optics", 30}, {1, "optics", "Optics", 40},
		{0, "magnets", "Magnets", 90}, {1, "magnets", "Magnets", 80},
	}
	gaps := ClassGaps(cells, map[string]int{"current": 5})

	if gaps[0].TopicID != "current" {
		t.Fatalf("topic blocking 5 others should rank first, got %s", gaps[0].TopicID)
	}
	if gaps[len(gaps)-1].TopicID != "magnets" {
		t.Fatalf("strong topic should rank last, got %s", gaps[len(gaps)-1].TopicID)
	}
	g := gaps[0]
	if g.AvgMastery != 35 || g.BelowHalf != 2 || g.StudentsSeen != 2 || g.Downstream != 5 {
		t.Fatalf("unexpected gap %+v", g)
	}
	// (1 − 0.35) · (0.5 + 2/2) · (1 + 5/5) = 1.95
	if g.Priority != 1.95 {
		t.Fatalf("priority = %v, want 1.95", g.Priority)
	}
}

func TestStudentRowsUseOpaqueKeysWeakestFirst(t *testing.T) {
	rows := StudentRows([]string{"Asha", "Ravi"}, []MasteryCell{
		{0, "a", "A", 80}, {1, "a", "A", 20}, {1, "b", "B", 40},
	})
	if rows[0].Name != "Ravi" || rows[0].AvgMastery != 30 || rows[0].Key != "s1" {
		t.Fatalf("weakest student should be first with key s1, got %+v", rows[0])
	}
	if _, ok := rows[1].Mastery["b"]; ok {
		t.Fatal("a topic a student has no score on must stay empty, not 0")
	}
}

func TestClassSummaryNamesWeakestStudentsAndQuotes(t *testing.T) {
	students := StudentRows([]string{"Asha", "Ravi"}, []MasteryCell{{0, "a", "Current", 80}, {1, "a", "Current", 20}})
	gaps := ClassGaps([]MasteryCell{{0, "a", "Current", 80}, {1, "a", "Current", 20}}, nil)
	s := ClassSummary("10-B", students, gaps, []ClassMisconception{{"a", "Current", "current gets used up", 1, 0, []string{"the bulb eats it"}}})
	for _, want := range []string{"Current: class average 50%", "Weakest: Ravi, Asha", `"current gets used up" (Current): 1 students`, "the bulb eats it"} {
		if !strings.Contains(s, want) {
			t.Fatalf("summary missing %q:\n%s", want, s)
		}
	}
}
