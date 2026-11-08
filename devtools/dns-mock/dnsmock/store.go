// Package dnsmock is a programmable DNS server for dev and staging (no plane owns it — it plays
// the outside world's DNS): every component that needs
// "real" DNS (zones-service's ownership and routing checks today; ACME DNS-01 or anything else
// later) queries it exactly as it would query a real resolver, while records and failures are set
// through an HTTP API instead of a registrar.
//
// Three query front-ends share one resolution path (Store.Resolve), so a record answers the same
// way however it is asked: classic DNS over UDP/TCP (dig, net.Resolver), RFC 8484 DoH
// (/dns-query), and the Google-style JSON DoH (/resolve) zones-service's dnsLookup speaks.
//
// Resolution follows a recursive resolver's observable behaviour: CNAMEs are chased (the chain is
// returned, as a real resolver returns it), `*.` wildcards synthesize answers for names that don't
// exist, a name that exists without the asked type is NODATA rather than NXDOMAIN, and a name
// with no record at all (nor beneath it) is NXDOMAIN — or, when an upstream is configured, is
// forwarded there, so the mock can overlay a handful of names on top of real DNS. Faults
// (servfail, refused, nxdomain, timeout, delay) are injected per name to exercise error paths.
//
// This is test infrastructure, never a production resolver: no DNSSEC, no caching, no ACLs
// beyond the optional admin token.
package dnsmock

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

const (
	defaultTTL    = 60
	maxCNAMEHops  = 8
	queryLogLimit = 500
	// timeoutHold: how long a "timeout" fault holds an HTTP (DoH) request open — longer than any
	// sane client timeout, so the caller's own timeout is what fires.
	timeoutHold = 30 * time.Second
)

// Record is one resource record. Value is the RDATA in zone-file syntax ("1.2.3.4",
// "target.example.", "10 mail.example."), except TXT, which takes the raw text (quoting and
// 255-byte splitting are done here).
type Record struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
	TTL   uint32 `json:"ttl"`
}

// FaultKind is the failure a matching query gets instead of (or, for delay, before) its answer.
type FaultKind string

const (
	FaultServfail FaultKind = "servfail"
	FaultRefused  FaultKind = "refused"
	FaultNXDomain FaultKind = "nxdomain"
	FaultTimeout  FaultKind = "timeout" // never answers
	FaultDelay    FaultKind = "delay"   // answers normally after DelayMs
)

// Fault applies to queries for Name (exact, or a `*.` wildcard), optionally only for one Type.
type Fault struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Type    string    `json:"type,omitempty"`
	Kind    FaultKind `json:"kind"`
	DelayMs int       `json:"delayMs,omitempty"`
}

// QueryLogEntry is one answered (or dropped) query, newest last in Store.Queries.
type QueryLogEntry struct {
	Time    time.Time `json:"time"`
	Proto   string    `json:"proto"`
	Name    string    `json:"name"`
	Type    string    `json:"type"`
	Rcode   string    `json:"rcode"`
	Answers int       `json:"answers"`
	Source  string    `json:"source"` // mock | upstream | fault | none
}

// Answer is the outcome of Resolve. Drop means "send nothing" (timeout fault).
type Answer struct {
	Rcode  int
	RRs    []dns.RR
	Source string
	Drop   bool
}

// Forwarder answers a query the mock doesn't own (Upstream below); nil means no forwarding.
type Forwarder func(ctx context.Context, name string, qtype uint16) (*dns.Msg, error)

type Store struct {
	mu      sync.RWMutex
	records []Record
	faults  []Fault
	queries []QueryLogEntry

	statePath string
	forward   Forwarder
}

type state struct {
	Records []Record `json:"records"`
	Faults  []Fault  `json:"faults"`
}

// NewStore loads statePath if it exists (every change is written back to it), otherwise seeds
// from seedPath if given. Both may be empty: an in-memory mock that starts blank.
func NewStore(statePath, seedPath string, forward Forwarder) (*Store, error) {
	s := &Store{statePath: statePath, forward: forward}
	loaded := false
	if statePath != "" {
		ok, err := s.loadFile(statePath)
		if err != nil {
			return nil, fmt.Errorf("loading state %s: %w", statePath, err)
		}
		loaded = ok
	}
	if !loaded && seedPath != "" {
		ok, err := s.loadFile(seedPath)
		if err != nil {
			return nil, fmt.Errorf("loading seed %s: %w", seedPath, err)
		}
		if !ok {
			return nil, fmt.Errorf("seed file %s does not exist", seedPath)
		}
		if err := s.persistLocked(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) loadFile(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var st state
	if err := json.Unmarshal(data, &st); err != nil {
		return false, err
	}
	for _, r := range st.Records {
		if _, err := s.addRecordLocked(r, false); err != nil {
			return false, fmt.Errorf("record %s %s: %w", r.Name, r.Type, err)
		}
	}
	for _, f := range st.Faults {
		if _, err := s.addFaultLocked(f); err != nil {
			return false, fmt.Errorf("fault %s: %w", f.Name, err)
		}
	}
	return true, nil
}

func (s *Store) persistLocked() error {
	if s.statePath == "" {
		return nil
	}
	data, err := json.MarshalIndent(state{Records: s.records, Faults: s.faults}, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.statePath + ".tmp"
	if err := os.MkdirAll(filepath.Dir(s.statePath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.statePath)
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// normalizeName lowercases and makes a name fully qualified, so "WWW.Shop.test" and
// "www.shop.test." are one name.
func normalizeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("name is required")
	}
	name = dns.CanonicalName(name)
	if _, ok := dns.IsDomainName(name); !ok {
		return "", fmt.Errorf("%q is not a valid domain name", name)
	}
	return name, nil
}

func parseType(t string) (uint16, error) {
	t = strings.ToUpper(strings.TrimSpace(t))
	if t == "" {
		return 0, errors.New("type is required")
	}
	if v, ok := dns.StringToType[t]; ok {
		return v, nil
	}
	return 0, fmt.Errorf("unknown record type %q", t)
}

func quoteTXT(v string) string {
	if strings.HasPrefix(strings.TrimSpace(v), `"`) {
		return v // already zone-file quoted by the caller
	}
	var parts []string
	for len(v) > 255 {
		parts = append(parts, v[:255])
		v = v[255:]
	}
	parts = append(parts, v)
	for i, p := range parts {
		p = strings.ReplaceAll(p, `\`, `\\`)
		parts[i] = `"` + strings.ReplaceAll(p, `"`, `\"`) + `"`
	}
	return strings.Join(parts, " ")
}

// toRR builds the wire RR for r, which is also how a record is validated on the way in.
func toRR(r Record) (dns.RR, error) {
	value := r.Value
	if r.Type == "TXT" || r.Type == "SPF" {
		value = quoteTXT(value)
	}
	rr, err := dns.NewRR(fmt.Sprintf("%s %d IN %s %s", r.Name, r.TTL, r.Type, value))
	if err != nil {
		return nil, fmt.Errorf("invalid %s value %q: %w", r.Type, r.Value, err)
	}
	if rr == nil {
		return nil, fmt.Errorf("empty %s value", r.Type)
	}
	return rr, nil
}

func (s *Store) addRecordLocked(r Record, replace bool) (Record, error) {
	name, err := normalizeName(r.Name)
	if err != nil {
		return Record{}, err
	}
	qtype, err := parseType(r.Type)
	if err != nil {
		return Record{}, err
	}
	r.Name, r.Type = name, dns.TypeToString[qtype]
	if r.TTL == 0 {
		r.TTL = defaultTTL
	}
	if _, err := toRR(r); err != nil {
		return Record{}, err
	}
	// A CNAME owns its name outright (RFC 1034 §3.6.2) — refuse the mix a real zone would.
	for _, existing := range s.records {
		if existing.Name != r.Name || (replace && existing.Type == r.Type) {
			continue
		}
		if (r.Type == "CNAME") != (existing.Type == "CNAME") {
			return Record{}, fmt.Errorf("%s already has a %s record; a CNAME cannot coexist with other records", r.Name, existing.Type)
		}
		if r.Type == "CNAME" && existing.Type == "CNAME" {
			return Record{}, fmt.Errorf("%s already has a CNAME; use replace to change it", r.Name)
		}
	}
	if replace {
		kept := s.records[:0]
		for _, existing := range s.records {
			if !(existing.Name == r.Name && existing.Type == r.Type) {
				kept = append(kept, existing)
			}
		}
		s.records = kept
	}
	if r.ID == "" {
		r.ID = newID()
	}
	s.records = append(s.records, r)
	return r, nil
}

// AddRecord adds r; with replace, every existing record of the same name and type goes first
// (the "set this TXT to exactly this token" case).
func (s *Store) AddRecord(r Record, replace bool) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out, err := s.addRecordLocked(r, replace)
	if err != nil {
		return Record{}, err
	}
	return out, s.persistLocked()
}

// DeleteRecords removes by id, or by name (and type, when given). Returns how many went.
func (s *Store) DeleteRecords(id, name, rtype string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if name != "" {
		n, err := normalizeName(name)
		if err != nil {
			return 0, err
		}
		name = n
	}
	rtype = strings.ToUpper(rtype)
	kept := s.records[:0]
	removed := 0
	for _, r := range s.records {
		match := (id != "" && r.ID == id) || (id == "" && name != "" && r.Name == name && (rtype == "" || r.Type == rtype))
		if match {
			removed++
			continue
		}
		kept = append(kept, r)
	}
	s.records = kept
	return removed, s.persistLocked()
}

func (s *Store) addFaultLocked(f Fault) (Fault, error) {
	name, err := normalizeName(f.Name)
	if err != nil {
		return Fault{}, err
	}
	f.Name = name
	if f.Type != "" {
		qtype, err := parseType(f.Type)
		if err != nil {
			return Fault{}, err
		}
		f.Type = dns.TypeToString[qtype]
	}
	switch f.Kind {
	case FaultServfail, FaultRefused, FaultNXDomain, FaultTimeout:
	case FaultDelay:
		if f.DelayMs <= 0 {
			return Fault{}, errors.New("delay fault needs delayMs > 0")
		}
	default:
		return Fault{}, fmt.Errorf("unknown fault kind %q (servfail, refused, nxdomain, timeout, delay)", f.Kind)
	}
	if f.ID == "" {
		f.ID = newID()
	}
	s.faults = append(s.faults, f)
	return f, nil
}

func (s *Store) AddFault(f Fault) (Fault, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out, err := s.addFaultLocked(f)
	if err != nil {
		return Fault{}, err
	}
	return out, s.persistLocked()
}

func (s *Store) DeleteFaults(id, name string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if name != "" {
		n, err := normalizeName(name)
		if err != nil {
			return 0, err
		}
		name = n
	}
	kept := s.faults[:0]
	removed := 0
	for _, f := range s.faults {
		if (id != "" && f.ID == id) || (id == "" && name != "" && f.Name == name) {
			removed++
			continue
		}
		kept = append(kept, f)
	}
	s.faults = kept
	return removed, s.persistLocked()
}

// Reset drops every record and fault (the query log too).
func (s *Store) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records, s.faults, s.queries = nil, nil, nil
	return s.persistLocked()
}

func (s *Store) Records() []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := append([]Record(nil), s.records...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Type < out[j].Type
	})
	return out
}

func (s *Store) Faults() []Fault {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Fault(nil), s.faults...)
}

func (s *Store) Queries() []QueryLogEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]QueryLogEntry(nil), s.queries...)
}

func (s *Store) logQuery(e QueryLogEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queries = append(s.queries, e)
	if len(s.queries) > queryLogLimit {
		s.queries = s.queries[len(s.queries)-queryLogLimit:]
	}
}

// wildcardNames lists the `*.` owners that could cover name, closest first:
// a.b.shop.test. → *.b.shop.test., *.shop.test., *.test.
func wildcardNames(name string) []string {
	labels := dns.SplitDomainName(name)
	var out []string
	for i := 1; i < len(labels); i++ {
		out = append(out, "*."+strings.Join(labels[i:], ".")+".")
	}
	return out
}

func (s *Store) faultFor(name string, qtype uint16) *Fault {
	candidates := append([]string{name}, wildcardNames(name)...)
	for _, c := range candidates {
		for i := range s.faults {
			f := s.faults[i]
			if f.Name == c && (f.Type == "" || f.Type == dns.TypeToString[qtype]) {
				return &f
			}
		}
	}
	return nil
}

// nameExists: the name owns a record, or something beneath it does (an "empty non-terminal",
// e.g. shop.test. when only _waf-verify.shop.test. is set) — NODATA territory, not NXDOMAIN.
func (s *Store) nameExists(name string) bool {
	for _, r := range s.records {
		if r.Name == name || strings.HasSuffix(r.Name, "."+name) {
			return true
		}
	}
	return false
}

// recordsAt returns the records owning name — its own, or a covering wildcard's (only when the
// name doesn't exist itself, as RFC 4592 has it), rewritten to name.
func (s *Store) recordsAt(name string) []Record {
	var own []Record
	for _, r := range s.records {
		if r.Name == name {
			own = append(own, r)
		}
	}
	if len(own) > 0 || s.nameExists(name) {
		return own
	}
	for _, w := range wildcardNames(name) {
		var matched []Record
		for _, r := range s.records {
			if r.Name == w {
				r.Name = name
				matched = append(matched, r)
			}
		}
		if len(matched) > 0 {
			return matched
		}
	}
	return nil
}

func rrsOf(records []Record, qtype uint16) []dns.RR {
	var out []dns.RR
	for _, r := range records {
		if qtype != dns.TypeANY && r.Type != dns.TypeToString[qtype] {
			continue
		}
		if rr, err := toRR(r); err == nil {
			out = append(out, rr)
		}
	}
	return out
}

// Resolve answers one question the way a recursive resolver would, from the mock's records,
// its faults, or (for names it doesn't own) the forwarder.
func (s *Store) Resolve(ctx context.Context, proto, rawName string, qtype uint16) Answer {
	name, err := normalizeName(rawName)
	if err != nil {
		return s.finish(proto, rawName, qtype, Answer{Rcode: dns.RcodeFormatError, Source: "none"})
	}

	s.mu.RLock()
	fault := s.faultFor(name, qtype)
	s.mu.RUnlock()
	if fault != nil {
		switch fault.Kind {
		case FaultServfail:
			return s.finish(proto, name, qtype, Answer{Rcode: dns.RcodeServerFailure, Source: "fault"})
		case FaultRefused:
			return s.finish(proto, name, qtype, Answer{Rcode: dns.RcodeRefused, Source: "fault"})
		case FaultNXDomain:
			return s.finish(proto, name, qtype, Answer{Rcode: dns.RcodeNameError, Source: "fault"})
		case FaultTimeout:
			return s.finish(proto, name, qtype, Answer{Drop: true, Source: "fault"})
		case FaultDelay:
			select {
			case <-time.After(time.Duration(fault.DelayMs) * time.Millisecond):
			case <-ctx.Done():
				return s.finish(proto, name, qtype, Answer{Drop: true, Source: "fault"})
			}
		}
	}

	ans := s.resolveRecords(ctx, name, qtype)
	return s.finish(proto, name, qtype, ans)
}

func (s *Store) resolveRecords(ctx context.Context, name string, qtype uint16) Answer {
	var chain []dns.RR
	seen := map[string]bool{}
	current := name
	for hop := 0; ; hop++ {
		s.mu.RLock()
		records := s.recordsAt(current)
		exists := len(records) > 0 || s.nameExists(current)
		s.mu.RUnlock()

		if !exists {
			if s.forward != nil {
				msg, err := s.forward(ctx, current, qtype)
				if err != nil {
					return Answer{Rcode: dns.RcodeServerFailure, RRs: chain, Source: "upstream"}
				}
				return Answer{Rcode: msg.Rcode, RRs: append(chain, msg.Answer...), Source: "upstream"}
			}
			if len(chain) > 0 {
				// A dangling CNAME: the chain so far, and NXDOMAIN for its target, as real
				// resolvers report it.
				return Answer{Rcode: dns.RcodeNameError, RRs: chain, Source: "mock"}
			}
			return Answer{Rcode: dns.RcodeNameError, Source: "none"}
		}

		if qtype != dns.TypeCNAME && qtype != dns.TypeANY {
			if cname := rrsOf(records, dns.TypeCNAME); len(cname) > 0 {
				chain = append(chain, cname[0])
				target := dns.CanonicalName(cname[0].(*dns.CNAME).Target)
				if seen[target] || hop >= maxCNAMEHops {
					return Answer{Rcode: dns.RcodeServerFailure, RRs: chain, Source: "mock"}
				}
				seen[current] = true
				current = target
				continue
			}
		}
		return Answer{Rcode: dns.RcodeSuccess, RRs: append(chain, rrsOf(records, qtype)...), Source: "mock"}
	}
}

func (s *Store) finish(proto, name string, qtype uint16, a Answer) Answer {
	rcode := dns.RcodeToString[a.Rcode]
	if a.Drop {
		rcode = "TIMEOUT"
	}
	s.logQuery(QueryLogEntry{
		Time: time.Now().UTC(), Proto: proto, Name: name, Type: dns.TypeToString[qtype],
		Rcode: rcode, Answers: len(a.RRs), Source: a.Source,
	})
	return a
}

// Upstream forwards to a real resolver (host:port), over UDP with a TCP retry on truncation.
func Upstream(addr string) Forwarder {
	return func(ctx context.Context, name string, qtype uint16) (*dns.Msg, error) {
		m := new(dns.Msg)
		m.SetQuestion(name, qtype)
		m.RecursionDesired = true
		c := &dns.Client{Net: "udp", Timeout: 3 * time.Second}
		resp, _, err := c.ExchangeContext(ctx, m, addr)
		if err == nil && resp.Truncated {
			c.Net = "tcp"
			resp, _, err = c.ExchangeContext(ctx, m, addr)
		}
		return resp, err
	}
}
