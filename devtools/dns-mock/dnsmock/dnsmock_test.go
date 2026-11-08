package dnsmock

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

func newStore(t *testing.T, records ...Record) *Store {
	t.Helper()
	s, err := NewStore("", "", nil)
	require.NoError(t, err)
	for _, r := range records {
		_, err := s.AddRecord(r, false)
		require.NoError(t, err)
	}
	return s
}

func resolve(s *Store, name string, qtype uint16) Answer {
	return s.Resolve(context.Background(), "test", name, qtype)
}

func values(a Answer) []string {
	var out []string
	for _, rr := range a.RRs {
		out = append(out, strings.TrimPrefix(rr.String(), rr.Header().String()))
	}
	return out
}

func TestResolveSemantics(t *testing.T) {
	s := newStore(t,
		Record{Name: "_waf-verify.shop.test", Type: "TXT", Value: "waf-verify-abc"},
		Record{Name: "www.shop.test", Type: "CNAME", Value: "z1.edge.waf.local."},
		Record{Name: "*.edge.waf.local", Type: "A", Value: "127.0.0.1"},
		Record{Name: "Shop.Test.", Type: "MX", Value: "10 mail.shop.test."},
	)

	a := resolve(s, "_WAF-VERIFY.shop.test.", dns.TypeTXT)
	require.Equal(t, dns.RcodeSuccess, a.Rcode)
	require.Equal(t, []string{`"waf-verify-abc"`}, values(a), "names are case-insensitive, TXT is quoted on the wire")

	a = resolve(s, "www.shop.test", dns.TypeA)
	require.Equal(t, dns.RcodeSuccess, a.Rcode)
	require.Equal(t, []string{"z1.edge.waf.local.", "127.0.0.1"}, values(a), "CNAME chased through a wildcard, chain returned")
	require.Equal(t, "z1.edge.waf.local.", a.RRs[1].Header().Name, "wildcard answer is owned by the asked name")

	a = resolve(s, "www.shop.test", dns.TypeCNAME)
	require.Equal(t, []string{"z1.edge.waf.local."}, values(a), "a CNAME question gets the CNAME itself")

	a = resolve(s, "shop.test", dns.TypeA)
	require.Equal(t, dns.RcodeSuccess, a.Rcode)
	require.Empty(t, a.RRs, "existing name without the type is NODATA")

	a = resolve(s, "missing.shop.test", dns.TypeA)
	require.Equal(t, dns.RcodeNameError, a.Rcode)
}

func TestCNAMEEdgeCases(t *testing.T) {
	s := newStore(t,
		Record{Name: "a.test", Type: "CNAME", Value: "b.test."},
		Record{Name: "b.test", Type: "CNAME", Value: "a.test."},
		Record{Name: "dangling.test", Type: "CNAME", Value: "nowhere.test."},
	)
	a := resolve(s, "a.test", dns.TypeA)
	require.Equal(t, dns.RcodeServerFailure, a.Rcode, "a CNAME loop is SERVFAIL, not a hang")

	a = resolve(s, "dangling.test", dns.TypeA)
	require.Equal(t, dns.RcodeNameError, a.Rcode)
	require.Equal(t, []string{"nowhere.test."}, values(a))

	_, err := s.AddRecord(Record{Name: "a.test", Type: "A", Value: "1.2.3.4"}, false)
	require.Error(t, err, "CNAME and other data can't share a name")
	_, err = s.AddRecord(Record{Name: "a.test", Type: "CNAME", Value: "c.test."}, true)
	require.NoError(t, err, "replace swaps the CNAME")
}

func TestValidationAndReplace(t *testing.T) {
	s := newStore(t)
	for _, bad := range []Record{
		{Name: "x.test", Type: "A", Value: "not-an-ip"},
		{Name: "x.test", Type: "BOGUS", Value: "1"},
		{Name: "", Type: "A", Value: "1.2.3.4"},
		{Name: "x.test", Type: "MX", Value: "mail.test."},
	} {
		_, err := s.AddRecord(bad, false)
		require.Error(t, err, "%+v", bad)
	}

	_, err := s.AddRecord(Record{Name: "t.test", Type: "TXT", Value: "old"}, false)
	require.NoError(t, err)
	_, err = s.AddRecord(Record{Name: "t.test", Type: "TXT", Value: "second"}, false)
	require.NoError(t, err)
	require.Len(t, values(resolve(s, "t.test", dns.TypeTXT)), 2)
	_, err = s.AddRecord(Record{Name: "t.test", Type: "TXT", Value: `has "quotes" and 300 chars ` + strings.Repeat("x", 300)}, true)
	require.NoError(t, err)
	got := resolve(s, "t.test", dns.TypeTXT)
	require.Len(t, got.RRs, 1)
	require.Len(t, got.RRs[0].(*dns.TXT).Txt, 2, "long TXT split into 255-byte strings")
	// miekg keeps TXT strings zone-escaped in memory; the wire carries the raw bytes.
	wire, err := got.RRs[0].(*dns.TXT).Txt, error(nil)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(strings.Join(wire, ""), `has \"quotes\"`))
	packed := new(dns.Msg)
	packed.Answer = got.RRs
	buf, err := packed.Pack()
	require.NoError(t, err)
	require.Contains(t, string(buf), `has "quotes" and`)

	n, err := s.DeleteRecords("", "T.TEST", "txt")
	require.NoError(t, err)
	require.Equal(t, 1, n)
}

func TestFaults(t *testing.T) {
	s := newStore(t, Record{Name: "ok.test", Type: "A", Value: "1.2.3.4"})
	for _, f := range []Fault{
		{Name: "sf.test", Kind: FaultServfail},
		{Name: "*.refused.test", Kind: FaultRefused},
		{Name: "ok.test", Type: "AAAA", Kind: FaultNXDomain},
		{Name: "drop.test", Kind: FaultTimeout},
		{Name: "slow.ok.test", Kind: FaultDelay, DelayMs: 50},
	} {
		_, err := s.AddFault(f)
		require.NoError(t, err)
	}
	_, err := s.AddFault(Fault{Name: "x.test", Kind: "explode"})
	require.Error(t, err)

	require.Equal(t, dns.RcodeServerFailure, resolve(s, "sf.test", dns.TypeA).Rcode)
	require.Equal(t, dns.RcodeRefused, resolve(s, "a.b.refused.test", dns.TypeTXT).Rcode)
	require.Equal(t, dns.RcodeNameError, resolve(s, "ok.test", dns.TypeAAAA).Rcode, "type-scoped fault")
	require.Equal(t, dns.RcodeSuccess, resolve(s, "ok.test", dns.TypeA).Rcode, "other types unaffected")
	require.True(t, resolve(s, "drop.test", dns.TypeA).Drop)

	start := time.Now()
	resolve(s, "slow.ok.test", dns.TypeA)
	require.GreaterOrEqual(t, time.Since(start), 50*time.Millisecond)

	q := s.Queries()
	require.NotEmpty(t, q)
	require.Equal(t, "fault", q[0].Source)
}

func TestUpstreamOverlay(t *testing.T) {
	var asked []string
	forward := func(ctx context.Context, name string, qtype uint16) (*dns.Msg, error) {
		asked = append(asked, name)
		m := new(dns.Msg)
		rr, _ := dns.NewRR(name + " 30 IN A 9.9.9.9")
		m.Answer = []dns.RR{rr}
		return m, nil
	}
	s, err := NewStore("", "", forward)
	require.NoError(t, err)
	_, err = s.AddRecord(Record{Name: "_waf-verify.real.example", Type: "TXT", Value: "tok"}, false)
	require.NoError(t, err)
	_, err = s.AddRecord(Record{Name: "www.real.example", Type: "CNAME", Value: "cdn.elsewhere.example."}, false)
	require.NoError(t, err)

	require.Equal(t, []string{`"tok"`}, values(resolve(s, "_waf-verify.real.example", dns.TypeTXT)))
	require.Empty(t, asked, "an owned name never reaches upstream")

	a := resolve(s, "api.other.example", dns.TypeA)
	require.Equal(t, "upstream", a.Source)
	require.Equal(t, []string{"9.9.9.9"}, values(a))

	a = resolve(s, "www.real.example", dns.TypeA)
	require.Equal(t, []string{"cdn.elsewhere.example.", "9.9.9.9"}, values(a), "a CNAME out of the mock is chased upstream")
}

func TestStatePersistsAndSeeds(t *testing.T) {
	dir := t.TempDir()
	seed := filepath.Join(dir, "seed.json")
	statePath := filepath.Join(dir, "state", "state.json")
	seeder, err := NewStore(seed, "", nil)
	require.NoError(t, err)
	_, err = seeder.AddRecord(Record{Name: "seeded.test", Type: "A", Value: "10.0.0.1"}, false)
	require.NoError(t, err)

	s, err := NewStore(statePath, seed, nil)
	require.NoError(t, err)
	require.Len(t, s.Records(), 1, "seeded on first boot")
	_, err = s.AddRecord(Record{Name: "added.test", Type: "A", Value: "10.0.0.2"}, false)
	require.NoError(t, err)

	again, err := NewStore(statePath, seed, nil)
	require.NoError(t, err)
	require.Len(t, again.Records(), 2, "state file wins over the seed once it exists")
}

// The three front-ends, end to end over real sockets.
func TestFrontEnds(t *testing.T) {
	s := newStore(t, Record{Name: "_waf-verify.shop.test", Type: "TXT", Value: "waf-verify-xyz"},
		Record{Name: "www.shop.test", Type: "CNAME", Value: "z9.edge.waf.local."})

	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	udp := &dns.Server{PacketConn: pc, Handler: DNSHandler(s)}
	go func() { _ = udp.ActivateAndServe() }()
	t.Cleanup(func() { _ = udp.Shutdown() })
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	tcp := &dns.Server{Listener: ln, Handler: DNSHandler(s)}
	go func() { _ = tcp.ActivateAndServe() }()
	t.Cleanup(func() { _ = tcp.Shutdown() })

	for _, c := range []struct{ net, addr string }{{"udp", pc.LocalAddr().String()}, {"tcp", ln.Addr().String()}} {
		m := new(dns.Msg)
		m.SetQuestion("_waf-verify.shop.test.", dns.TypeTXT)
		var resp *dns.Msg
		require.Eventually(t, func() bool {
			resp, _, err = (&dns.Client{Net: c.net, Timeout: time.Second}).Exchange(m, c.addr)
			return err == nil
		}, 3*time.Second, 50*time.Millisecond, c.net)
		require.Equal(t, dns.RcodeSuccess, resp.Rcode)
		require.True(t, resp.Authoritative)
		require.Equal(t, []string{"waf-verify-xyz"}, resp.Answer[0].(*dns.TXT).Txt, c.net)
	}

	// Go's own resolver pointed at the mock — what any service using net.Resolver would see.
	r := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "udp", pc.LocalAddr().String())
	}}
	txt, err := r.LookupTXT(context.Background(), "_waf-verify.shop.test")
	require.NoError(t, err)
	require.Equal(t, []string{"waf-verify-xyz"}, txt)

	srv := httptest.NewServer(HTTPHandler(s, "secret"))
	t.Cleanup(srv.Close)

	// JSON DoH, Google's shape.
	resp, err := http.Get(srv.URL + "/resolve?name=www.shop.test&type=CNAME")
	require.NoError(t, err)
	var body struct {
		Status int
		Answer []struct {
			Name string `json:"name"`
			Type int    `json:"type"`
			Data string `json:"data"`
		}
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	resp.Body.Close()
	require.Equal(t, 0, body.Status)
	require.Equal(t, "z9.edge.waf.local.", body.Answer[0].Data)
	require.Equal(t, 5, body.Answer[0].Type)

	// RFC 8484 GET.
	q := new(dns.Msg)
	q.SetQuestion("missing.shop.test.", dns.TypeA)
	packed, _ := q.Pack()
	resp, err = http.Get(srv.URL + "/dns-query?dns=" + base64.RawURLEncoding.EncodeToString(packed))
	require.NoError(t, err)
	require.Equal(t, "application/dns-message", resp.Header.Get("Content-Type"))
	raw := make([]byte, 4096)
	n, _ := resp.Body.Read(raw)
	resp.Body.Close()
	out := new(dns.Msg)
	require.NoError(t, out.Unpack(raw[:n]))
	require.Equal(t, dns.RcodeNameError, out.Rcode)

	// Admin API: token enforced, write then read back through DNS.
	resp, err = http.Post(srv.URL+"/api/records", "application/json", strings.NewReader(`{"name":"new.test","type":"A","value":"1.1.1.1"}`))
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/records", strings.NewReader(`{"name":"new.test","type":"A","value":"1.1.1.1"}`))
	req.Header.Set("Authorization", "Bearer secret")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	ips, err := r.LookupHost(context.Background(), "new.test")
	require.NoError(t, err)
	require.Equal(t, []string{"1.1.1.1"}, ips)

	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/api/records", strings.NewReader(`{"name":"bad.test","type":"A","value":"nope"}`))
	req.Header.Set("Authorization", "Bearer secret")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// Every common record type goes in through the same zone-file parser and comes back out over the
// wire unchanged.
func TestRecordTypes(t *testing.T) {
	s := newStore(t,
		Record{Name: "v4.test", Type: "A", Value: "203.0.113.10"},
		Record{Name: "v6.test", Type: "AAAA", Value: "2001:db8::10"},
		Record{Name: "dual.test", Type: "A", Value: "203.0.113.11"},
		Record{Name: "dual.test", Type: "AAAA", Value: "2001:db8::11"},
		Record{Name: "alias.test", Type: "CNAME", Value: "dual.test."},
		Record{Name: "shop.test", Type: "NS", Value: "ns1.provider.test."},
		Record{Name: "shop.test", Type: "CAA", Value: `0 issue "letsencrypt.org"`},
		Record{Name: "_https._tcp.shop.test", Type: "SRV", Value: "10 5 443 www.shop.test."},
		Record{Name: "10.113.0.203.in-addr.arpa", Type: "PTR", Value: "v4.test."},
	)
	require.Equal(t, []string{"203.0.113.10"}, values(resolve(s, "v4.test", dns.TypeA)))
	require.Equal(t, []string{"2001:db8::10"}, values(resolve(s, "v6.test", dns.TypeAAAA)))
	require.Empty(t, resolve(s, "v6.test", dns.TypeA).RRs, "AAAA-only name has no A (NODATA)")
	require.Equal(t, []string{"dual.test.", "2001:db8::11"}, values(resolve(s, "alias.test", dns.TypeAAAA)), "CNAME chased for AAAA too")
	require.Len(t, resolve(s, "dual.test", dns.TypeANY).RRs, 2)
	require.Equal(t, []string{"ns1.provider.test."}, values(resolve(s, "shop.test", dns.TypeNS)))
	require.Equal(t, []string{`0 issue "letsencrypt.org"`}, values(resolve(s, "shop.test", dns.TypeCAA)))
	require.Equal(t, []string{"10 5 443 www.shop.test."}, values(resolve(s, "_https._tcp.shop.test", dns.TypeSRV)))
	require.Equal(t, []string{"v4.test."}, values(resolve(s, "10.113.0.203.in-addr.arpa", dns.TypePTR)))

	_, err := s.AddRecord(Record{Name: "bad.test", Type: "AAAA", Value: "203.0.113.1"}, false)
	require.Error(t, err, "an IPv4 address is not an AAAA value")
	_, err = s.AddRecord(Record{Name: "bad.test", Type: "A", Value: "2001:db8::1"}, false)
	require.Error(t, err, "an IPv6 address is not an A value")
}
