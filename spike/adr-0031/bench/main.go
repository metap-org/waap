// Open-loop load generator for the ADR-0031 spike. Latency is measured from the intended send
// time, so a stalled server cannot hide its own queueing (coordinated omission).
package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type sample struct {
	at  time.Duration // since start
	lat time.Duration
	st  int
	err bool
}

func pct(s []time.Duration, p float64) time.Duration {
	if len(s) == 0 {
		return 0
	}
	i := int(float64(len(s)-1) * p)
	return s[i]
}

func main() {
	url := flag.String("url", "http://127.0.0.1:8080/static/a.js", "target")
	host := flag.String("host", "", "Host header")
	rate := flag.Int("rate", 1000, "requests per second")
	dur := flag.Duration("dur", 20*time.Second, "duration")
	conns := flag.Int("conns", 64, "max connections")
	attack := flag.Float64("attack", 0, "fraction of requests carrying an attack payload")
	body := flag.Int("body", 0, "POST body bytes (0 = GET)")
	timeline := flag.Bool("timeline", false, "print per-second p99 and errors")
	ctype := flag.String("ctype", "", "Content-Type for POST; application/json builds a JSON body")
	idle := flag.Int("idle", 0, "open this many idle keep-alive connections first and hold them")
	flag.Parse()

	tr := &http.Transport{MaxIdleConns: *conns, MaxIdleConnsPerHost: *conns, MaxConnsPerHost: *conns, DisableCompression: true}
	cl := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	_ = idle

	var mu sync.Mutex
	var all []sample
	var sent atomic.Int64
	start := time.Now()
	interval := time.Second / time.Duration(*rate)
	payload := strings.Repeat("x", *body)
	if *ctype == "application/json" {
		var sb strings.Builder
		sb.WriteString("{")
		for i := 0; sb.Len() < *body; i++ {
			if i > 0 {
				sb.WriteString(",")
			}
			fmt.Fprintf(&sb, `"field%d":"hello world value %d"`, i, i)
		}
		sb.WriteString("}")
		payload = sb.String()
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, *conns*4)
	n := int(float64(*rate) * dur.Seconds())
	for i := 0; i < n; i++ {
		intended := start.Add(time.Duration(i) * interval)
		time.Sleep(time.Until(intended))
		sem <- struct{}{}
		wg.Add(1)
		sent.Add(1)
		isAttack := *attack > 0 && float64(i%1000)/1000 < *attack
		go func(i int, intended time.Time, isAttack bool) {
			defer func() { <-sem; wg.Done() }()
			u := *url
			if isAttack {
				sep := "?"
				if strings.Contains(u, "?") {
					sep = "&"
				}
				u += sep + "q=%27%20OR%201%3D1--%20-&x=%3Cscript%3Ealert(1)%3C%2Fscript%3E"
			}
			var req *http.Request
			if *body > 0 {
				req, _ = http.NewRequest("POST", u, strings.NewReader(payload))
			} else {
				req, _ = http.NewRequest("GET", u, nil)
			}
			if *ctype != "" {
				req.Header.Set("Content-Type", *ctype)
			}
			if *host != "" {
				req.Host = *host
			}
			resp, err := cl.Do(req)
			s := sample{at: time.Since(start)}
			if err != nil {
				s.err = true
			} else {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				s.st = resp.StatusCode
			}
			s.lat = time.Since(intended)
			mu.Lock()
			all = append(all, s)
			mu.Unlock()
		}(i, intended, isAttack)
	}
	wg.Wait()
	elapsed := time.Since(start)

	var lats []time.Duration
	codes := map[int]int{}
	errs := 0
	for _, s := range all {
		if s.err {
			errs++
			continue
		}
		codes[s.st]++
		lats = append(lats, s.lat)
	}
	sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })
	f := func(d time.Duration) string { return fmt.Sprintf("%.2fms", float64(d.Microseconds())/1000) }
	fmt.Printf("sent=%d errors=%d codes=%v elapsed=%.1fs achieved=%.0f rps\n", sent.Load(), errs, codes, elapsed.Seconds(), float64(len(all))/elapsed.Seconds())
	fmt.Printf("p50=%s p90=%s p99=%s p99.9=%s max=%s\n", f(pct(lats, .5)), f(pct(lats, .9)), f(pct(lats, .99)), f(pct(lats, .999)), f(pct(lats, 1)))
	if *timeline {
		buckets := map[int][]time.Duration{}
		berr := map[int]int{}
		for _, s := range all {
			b := int(s.at.Seconds())
			if s.err {
				berr[b]++
				continue
			}
			buckets[b] = append(buckets[b], s.lat)
		}
		for b := 0; b <= int(dur.Seconds()); b++ {
			l := buckets[b]
			sort.Slice(l, func(i, j int) bool { return l[i] < l[j] })
			fmt.Printf("t=%02ds n=%d p99=%s err=%d\n", b, len(l), f(pct(l, .99)), berr[b])
		}
	}
}
