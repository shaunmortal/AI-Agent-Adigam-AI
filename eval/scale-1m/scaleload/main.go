// Command scaleload drives the scale1m API (:4600) for a fixed duration with a learner mix:
// every request picks a fresh random learner out of 1,000,000 and sends that learner's token,
// ~70% reads across the main learner endpoints and ~30% POST /attempts. It writes one JSON
// file with per-endpoint and overall counts, throughput, latency percentiles and error kinds.
package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"sync"
	"time"
)

const (
	learners    = 1_000_000
	physicsID   = "0153f4d4-9f3d-4abe-8de0-34a72486ae55" // Class 10 Physics, every learner's goal
	correctRate = 0.6                                    // share of answers that pick the key
)

type endpoint struct {
	Name   string `json:"name"`
	Method string `json:"method"`
	Path   string `json:"path"`
	Weight int    `json:"weight"` // out of 100
}

// mix: reads sum to 70, the answer POST is 30.
var mix = []endpoint{
	{"GET /goals", "GET", "/goals", 10},
	{"GET /learner/summary", "GET", "/learner/summary", 10},
	{"GET /recommendations/next", "GET", "/recommendations/next?minutes=20", 14},
	{"GET /revision/due", "GET", "/revision/due", 8},
	{"GET /readiness", "GET", "/readiness", 7},
	{"GET /learner/activity", "GET", "/learner/activity", 7},
	{"GET /learner/subjects", "GET", "/learner/subjects", 5},
	{"GET /learner/profile", "GET", "/learner/profile", 4},
	{"GET /classes", "GET", "/classes", 3},
	{"GET /sources/map", "GET", "/sources/map/" + physicsID, 2},
	{"POST /attempts", "POST", "/attempts", 30},
}

type question struct {
	id      string
	correct int
}

type sample struct {
	ep   int
	dur  time.Duration
	kind string // "201", "503 SERVER_BUSY", "transport: ...", "200 success!=true", ...
	ok   bool
	at   time.Duration // when the request started, since the run started
}

type stats struct {
	Requests  int            `json:"requests"`
	Errors    int            `json:"errors"`
	RPS       float64        `json:"rps"`
	MeanMs    float64        `json:"meanMs"`
	P50Ms     float64        `json:"p50Ms"`
	P95Ms     float64        `json:"p95Ms"`
	P99Ms     float64        `json:"p99Ms"`
	MaxMs     float64        `json:"maxMs"`
	Responses map[string]int `json:"responses"` // status (and error code when not 2xx) -> count
	// ErrorWindowS: per error kind, seconds into the run of its first and last occurrence.
	ErrorWindowS map[string][2]float64 `json:"errorWindowS,omitempty"`
}

type report struct {
	Label       string           `json:"label"`
	Base        string           `json:"base"`
	Concurrency int              `json:"concurrency"`
	DurationS   float64          `json:"durationS"`
	StartedAt   time.Time        `json:"startedAt"`
	ElapsedS    float64          `json:"elapsedS"` // includes draining requests in flight at the deadline
	Seed        uint64           `json:"seed"`
	Mix         []endpoint       `json:"mix"`
	Overall     stats            `json:"overall"`
	Endpoints   map[string]stats `json:"endpoints"`
}

func main() {
	base := flag.String("base", "http://localhost:4600/api/v1", "API base URL (localhost only)")
	conc := flag.Int("c", 32, "concurrent workers, one request in flight each")
	dur := flag.Duration("d", 60*time.Second, "run duration")
	qfile := flag.String("questions", "questions.csv", "question id,correct option index")
	out := flag.String("out", "", "JSON report path (stdout when empty)")
	label := flag.String("label", "", "run label")
	seed := flag.Uint64("seed", uint64(time.Now().UnixNano()), "random seed")
	flag.Parse()

	if u, err := url.Parse(*base); err != nil || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1") {
		fail("refusing: scaleload only targets localhost")
	}
	if *conc < 1 || *dur <= 0 {
		fail("-c must be >= 1 and -d > 0")
	}
	qs, err := loadQuestions(*qfile)
	if err != nil {
		fail(err.Error())
	}

	tr := &http.Transport{MaxIdleConns: *conc, MaxIdleConnsPerHost: *conc, MaxConnsPerHost: *conc,
		IdleConnTimeout: 90 * time.Second, DisableCompression: true}
	client := &http.Client{Transport: tr, Timeout: 30 * time.Second}

	var pick []int // endpoint index per percent
	for i, e := range mix {
		for range e.Weight {
			pick = append(pick, i)
		}
	}
	if len(pick) != 100 {
		fail(fmt.Sprintf("mix weights sum to %d, want 100", len(pick)))
	}

	results := make([][]sample, *conc)
	var wg sync.WaitGroup
	start := time.Now()
	deadline := start.Add(*dur)
	for w := range *conc {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rng := rand.New(rand.NewPCG(*seed, uint64(w)))
			var mine []sample
			for time.Now().Before(deadline) {
				ep := pick[rng.IntN(100)]
				at := time.Since(start)
				s := do(client, *base, ep, rng, qs)
				s.at = at
				mine = append(mine, s)
			}
			results[w] = mine
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	all := slices.Concat(results...)
	rep := report{Label: *label, Base: *base, Concurrency: *conc, DurationS: dur.Seconds(), StartedAt: start,
		ElapsedS: elapsed.Seconds(), Seed: *seed, Mix: mix, Overall: summarise(all, elapsed),
		Endpoints: map[string]stats{}}
	for i, e := range mix {
		rep.Endpoints[e.Name] = summarise(filter(all, i), elapsed)
	}
	b, _ := json.MarshalIndent(rep, "", "  ")
	if *out == "" {
		os.Stdout.Write(append(b, '\n'))
		return
	}
	if err := os.WriteFile(*out, append(b, '\n'), 0o644); err != nil {
		fail(err.Error())
	}
	o := rep.Overall
	fmt.Printf("%s c=%d n=%d rps=%.1f p50=%.2fms p95=%.2fms p99=%.2fms errors=%d %v\n",
		*label, *conc, o.Requests, o.RPS, o.P50Ms, o.P95Ms, o.P99Ms, o.Errors, o.Responses)
}

// do sends one request as a random learner and classifies the outcome.
func do(client *http.Client, base string, ep int, rng *rand.Rand, qs []question) sample {
	e := mix[ep]
	token := fmt.Sprintf("scale1m-learner-%07d", rng.IntN(learners)+1)
	var body io.Reader
	if e.Method == "POST" {
		q := qs[rng.IntN(len(qs))]
		opt := q.correct
		if rng.Float64() >= correctRate {
			opt = (q.correct + 1 + rng.IntN(3)) % 4 // one of the three wrong options
		}
		body = bytes.NewBufferString(fmt.Sprintf(`{"questionId":%q,"selectedOptions":[%d],"timeTakenSeconds":%d}`,
			q.id, opt, 10+rng.IntN(111)))
	}
	req, _ := http.NewRequest(e.Method, base+e.Path, body)
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	t := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return sample{ep: ep, dur: time.Since(t), kind: "transport: " + errKind(err)}
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	d := time.Since(t)
	code := strconv.Itoa(resp.StatusCode)
	if resp.StatusCode >= 300 {
		var env struct {
			Error struct{ Code string } `json:"error"`
		}
		_ = json.Unmarshal(b, &env)
		return sample{ep: ep, dur: d, kind: code + " " + env.Error.Code}
	}
	if !bytes.Contains(b, []byte(`"success":true`)) {
		return sample{ep: ep, dur: d, kind: code + " success!=true"}
	}
	return sample{ep: ep, dur: d, kind: code, ok: true}
}

func errKind(err error) string {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return "timeout"
	}
	var oe *net.OpError // drop the addresses so kinds group: "dial: connect: connection reset by peer"
	if errors.As(err, &oe) {
		return oe.Op + ": " + oe.Err.Error()
	}
	return err.Error()
}

func filter(all []sample, ep int) []sample {
	var out []sample
	for _, s := range all {
		if s.ep == ep {
			out = append(out, s)
		}
	}
	return out
}

func summarise(ss []sample, elapsed time.Duration) stats {
	st := stats{Requests: len(ss), Responses: map[string]int{}}
	if len(ss) == 0 {
		return st
	}
	durs := make([]time.Duration, len(ss))
	var sum time.Duration
	for i, s := range ss {
		durs[i] = s.dur
		sum += s.dur
		st.Responses[s.kind]++
		if !s.ok {
			st.Errors++
			if st.ErrorWindowS == nil {
				st.ErrorWindowS = map[string][2]float64{}
			}
			w, seen := st.ErrorWindowS[s.kind]
			t := s.at.Seconds()
			if !seen {
				w = [2]float64{t, t}
			}
			st.ErrorWindowS[s.kind] = [2]float64{min(w[0], t), max(w[1], t)}
		}
	}
	slices.Sort(durs)
	ms := func(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
	// nearest rank: the smallest value with at least p of the samples at or below it
	rank := func(p float64) float64 {
		i := int(float64(len(durs))*p+0.999999999) - 1
		return ms(durs[max(0, min(i, len(durs)-1))])
	}
	st.RPS = float64(len(ss)) / elapsed.Seconds()
	st.MeanMs = ms(sum / time.Duration(len(ss)))
	st.P50Ms, st.P95Ms, st.P99Ms, st.MaxMs = rank(0.50), rank(0.95), rank(0.99), ms(durs[len(durs)-1])
	return st
}

func loadQuestions(path string) ([]question, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	var qs []question
	for _, r := range rows {
		if len(r) != 2 {
			return nil, fmt.Errorf("bad question row %v", r)
		}
		c, err := strconv.Atoi(r[1])
		if err != nil || c < 0 || c > 3 {
			return nil, fmt.Errorf("bad question row %v", r)
		}
		qs = append(qs, question{r[0], c})
	}
	if len(qs) == 0 {
		return nil, fmt.Errorf("no questions in %s", path)
	}
	return qs, nil
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(2)
}
