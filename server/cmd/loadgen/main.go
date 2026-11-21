// Command loadgen fires a fixed number of GET requests at one URL from many goroutines over
// keep-alive connections and prints throughput, latency percentiles and status-code counts.
package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	target := flag.String("url", "http://localhost:4000/api/v1/health", "URL to GET")
	total := flag.Int("n", 1_000_000, "total requests")
	conc := flag.Int("c", 128, "concurrent workers")
	flag.Parse()
	if *total < 1 || *conc < 1 {
		fmt.Fprintln(os.Stderr, "-n and -c must be at least 1")
		os.Exit(2)
	}

	u, err := url.Parse(*target)
	if err != nil || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1") {
		fmt.Fprintln(os.Stderr, "refusing: loadgen only targets localhost")
		os.Exit(2)
	}

	tr := &http.Transport{MaxIdleConns: *conc, MaxIdleConnsPerHost: *conc, MaxConnsPerHost: *conc,
		IdleConnTimeout: 90 * time.Second, DisableCompression: true}
	client := &http.Client{Transport: tr, Timeout: 30 * time.Second}

	var next atomic.Int64
	lat := make([][]time.Duration, *conc)
	codes := make([]map[int]int, *conc)
	var wg sync.WaitGroup
	start := time.Now()
	for w := range *conc {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mine := make([]time.Duration, 0, *total / *conc + 1)
			cc := map[int]int{}
			for next.Add(1) <= int64(*total) {
				t := time.Now()
				resp, err := client.Get(*target)
				if err != nil {
					if cc[-1] == 0 {
						fmt.Fprintln(os.Stderr, "transport error:", err)
					}
					cc[-1]++ // transport error
					mine = append(mine, time.Since(t))
					continue
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				mine = append(mine, time.Since(t))
				cc[resp.StatusCode]++
			}
			lat[w], codes[w] = mine, cc
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	all := slices.Concat(lat...)
	sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })
	merged := map[int]int{}
	for _, cc := range codes {
		for k, v := range cc {
			merged[k] += v
		}
	}
	errs := 0
	for k, v := range merged {
		if k != 200 {
			errs += v
		}
	}
	pct := func(p float64) float64 {
		return float64(all[int(p*float64(len(all)-1))].Microseconds()) / 1000
	}
	fmt.Printf("url=%s n=%d c=%d elapsed=%.2fs rps=%.0f p50=%.3fms p95=%.3fms p99=%.3fms max=%.3fms errors=%d codes=%v\n",
		*target, len(all), *conc, elapsed.Seconds(), float64(len(all))/elapsed.Seconds(),
		pct(0.50), pct(0.95), pct(0.99), float64(all[len(all)-1].Microseconds())/1000, errs, merged)
}
