// WAF inspection service for the ADR-0031 spike: Coraza + OWASP CRS behind a gRPC callout.
// The ruleset is built in the background and swapped atomically (apply class B, ADR-0037);
// per-zone exceptions are applied per transaction with RemoveRuleByID, never by building
// a separate WAF instance per zone.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/corazawaf/coraza-coreruleset/v4"
	"github.com/corazawaf/coraza/v3"
	"github.com/corazawaf/coraza/v3/types"
	"google.golang.org/grpc"

	pb "spike/waf/inspectpb"
)

type ruleset struct {
	waf     coraza.WAF
	version uint64
}

type server struct {
	pb.UnimplementedInspectionServer
	cur      atomic.Pointer[ruleset]
	requests atomic.Uint64
	blocked  atomic.Uint64
}

func buildWAF(paranoia int) (coraza.WAF, error) {
	cfg := coraza.NewWAFConfig().
		WithRootFS(coreruleset.FS).
		WithDirectives(fmt.Sprintf(`
Include @coraza.conf-recommended
Include @crs-setup.conf.example
SecAction "id:900000,phase:1,pass,t:none,nolog,setvar:tx.blocking_paranoia_level=%d"
Include @owasp_crs/*.conf
SecRuleEngine On
`, paranoia))
	return coraza.NewWAF(cfg)
}

type ruleRemover interface{ RemoveRuleByID(id int) }

func (s *server) Inspect(_ context.Context, in *pb.InspectRequest) (*pb.Verdict, error) {
	rs := s.cur.Load()
	s.requests.Add(1)
	tx := rs.waf.NewTransaction()
	defer func() { tx.ProcessLogging(); _ = tx.Close() }()
	if rr, ok := tx.(ruleRemover); ok {
		for _, id := range in.ExcludeRuleIds {
			rr.RemoveRuleByID(int(id))
		}
	}
	tx.ProcessConnection(in.ClientIp, 0, "", 0)
	tx.ProcessURI(in.Uri, in.Method, "HTTP/"+in.HttpVersion)
	for _, h := range in.Headers {
		tx.AddRequestHeader(h.Name, h.Value)
		if h.Name == "Host" || h.Name == "host" {
			tx.SetServerName(h.Value)
		}
	}
	verdict := func(it *types.Interruption) *pb.Verdict {
		s.blocked.Add(1)
		if os.Getenv("WAF_DEBUG") != "" {
			ids := []int{}
			for _, m := range tx.MatchedRules() {
				ids = append(ids, m.Rule().ID())
			}
			log.Printf("block rule=%d status=%d uri=%s method=%s matched=%v", it.RuleID, it.Status, in.Uri, in.Method, ids)
		}
		return &pb.Verdict{Action: pb.Verdict_BLOCK, Status: uint32(it.Status), RuleIds: []uint32{uint32(it.RuleID)}, RulesetVersion: rs.version}
	}
	if it := tx.ProcessRequestHeaders(); it != nil {
		return verdict(it), nil
	}
	if len(in.Body) > 0 {
		if it, _, err := tx.WriteRequestBody(in.Body); err != nil {
			return nil, err
		} else if it != nil {
			return verdict(it), nil
		}
	}
	it, err := tx.ProcessRequestBody()
	if err != nil {
		return nil, err
	}
	if it != nil {
		return verdict(it), nil
	}
	return &pb.Verdict{Action: pb.Verdict_ALLOW, RulesetVersion: rs.version}, nil
}

func cpuSeconds() float64 {
	var ru syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
	return float64(ru.Utime.Sec+ru.Stime.Sec) + float64(ru.Utime.Usec+ru.Stime.Usec)/1e6
}

func main() {
	addr := flag.String("addr", "127.0.0.1:9100", "gRPC listen address")
	admin := flag.String("admin", "127.0.0.1:9101", "admin HTTP address")
	paranoia := flag.Int("paranoia", 1, "CRS blocking paranoia level")
	flag.Parse()

	s := &server{}
	t0 := time.Now()
	w, err := buildWAF(*paranoia)
	if err != nil {
		log.Fatal(err)
	}
	s.cur.Store(&ruleset{waf: w, version: 1})
	log.Printf("initial ruleset built in %v", time.Since(t0))

	mux := http.NewServeMux()
	mux.HandleFunc("/reload", func(rw http.ResponseWriter, r *http.Request) {
		p, _ := strconv.Atoi(r.URL.Query().Get("paranoia"))
		if p == 0 {
			p = *paranoia
		}
		var m0, m1 runtime.MemStats
		runtime.ReadMemStats(&m0)
		start := time.Now()
		nw, err := buildWAF(p)
		if err != nil {
			http.Error(rw, err.Error(), 500)
			return
		}
		build := time.Since(start)
		runtime.ReadMemStats(&m1)
		old := s.cur.Load()
		s.cur.Store(&ruleset{waf: nw, version: old.version + 1})
		json.NewEncoder(rw).Encode(map[string]any{"build_ms": build.Milliseconds(), "version": old.version + 1,
			"alloc_mb_during_build": (m1.TotalAlloc - m0.TotalAlloc) >> 20, "heap_mb_after": m1.HeapAlloc >> 20})
	})
	mux.HandleFunc("/stats", func(rw http.ResponseWriter, _ *http.Request) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		json.NewEncoder(rw).Encode(map[string]any{"requests": s.requests.Load(), "blocked": s.blocked.Load(),
			"cpu_seconds": cpuSeconds(), "heap_mb": m.HeapAlloc >> 20, "sys_mb": m.Sys >> 20, "gc_pause_total_ms": m.PauseTotalNs / 1e6, "ruleset_version": s.cur.Load().version})
	})
	// Benchmark-only control port on the bench host's loopback/private network; this spike is
	// never deployed, and TLS here would only skew the reload timings it exists to measure.
	go func() { log.Fatal(http.ListenAndServe(*admin, mux)) }() // nosemgrep: go.lang.security.audit.net.use-tls.use-tls

	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	g := grpc.NewServer(grpc.MaxRecvMsgSize(16 << 20))
	pb.RegisterInspectionServer(g, s)
	log.Printf("waf listening grpc=%s admin=%s paranoia=%d", *addr, *admin, *paranoia)
	log.Fatal(g.Serve(lis))
}
