package main

import (
	"testing"
	"time"
)

func TestSummariseNearestRank(t *testing.T) {
	var ss []sample
	for i := 1; i <= 100; i++ {
		ss = append(ss, sample{dur: time.Duration(i) * time.Millisecond, kind: "200", ok: i != 7})
	}
	st := summarise(ss, 10*time.Second)
	if st.P50Ms != 50 || st.P95Ms != 95 || st.P99Ms != 99 || st.MaxMs != 100 || st.RPS != 10 || st.Errors != 1 {
		t.Fatalf("got %+v", st)
	}
}
