package dnsmock

import (
	"context"
	"crypto/subtle"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"
)

//go:embed ui.html
var uiHTML []byte

// DNSHandler serves classic DNS (UDP and TCP) from the store.
func DNSHandler(s *Store) dns.Handler {
	return dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		proto := "udp"
		if w.LocalAddr() != nil && w.LocalAddr().Network() == "tcp" {
			proto = "tcp"
		}
		resp, drop := answerMsg(context.Background(), s, proto, r)
		if drop {
			return
		}
		if proto == "udp" {
			size := dns.MinMsgSize
			if opt := r.IsEdns0(); opt != nil {
				size = int(opt.UDPSize())
			}
			resp.Truncate(size)
		}
		_ = w.WriteMsg(resp)
	})
}

// answerMsg builds the reply to a wire-format query (shared by UDP/TCP and RFC 8484 DoH).
func answerMsg(ctx context.Context, s *Store, proto string, r *dns.Msg) (*dns.Msg, bool) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.RecursionAvailable = true
	if len(r.Question) != 1 {
		m.Rcode = dns.RcodeFormatError
		return m, false
	}
	q := r.Question[0]
	a := s.Resolve(ctx, proto, q.Name, q.Qtype)
	if a.Drop {
		return nil, true
	}
	m.Rcode = a.Rcode
	m.Answer = a.RRs
	m.Authoritative = a.Source == "mock"
	return m, false
}

// HTTPHandler serves the DoH front-ends, the admin API and the UI. adminToken, when non-empty,
// guards /api/* (Authorization: Bearer <token>); the query endpoints stay open, as a resolver is.
func HTTPHandler(s *Store, adminToken string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(uiHTML)
	})
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /resolve", func(w http.ResponseWriter, r *http.Request) { resolveJSON(w, r, s) })
	mux.HandleFunc("GET /dns-query", func(w http.ResponseWriter, r *http.Request) { dohWire(w, r, s) })
	mux.HandleFunc("POST /dns-query", func(w http.ResponseWriter, r *http.Request) { dohWire(w, r, s) })

	api := http.NewServeMux()
	api.HandleFunc("GET /api/records", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.Records())
	})
	api.HandleFunc("POST /api/records", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Record
			Replace bool `json:"replace"`
		}
		if !readJSON(w, r, &body) {
			return
		}
		rec, err := s.AddRecord(body.Record, body.Replace)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusCreated, rec)
	})
	api.HandleFunc("DELETE /api/records/{id}", func(w http.ResponseWriter, r *http.Request) {
		deleted(w)(s.DeleteRecords(r.PathValue("id"), "", ""))
	})
	api.HandleFunc("DELETE /api/records", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "" {
			writeError(w, http.StatusBadRequest, errors.New("name query parameter is required (POST /api/reset clears everything)"))
			return
		}
		deleted(w)(s.DeleteRecords("", name, r.URL.Query().Get("type")))
	})
	api.HandleFunc("GET /api/faults", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.Faults())
	})
	api.HandleFunc("POST /api/faults", func(w http.ResponseWriter, r *http.Request) {
		var f Fault
		if !readJSON(w, r, &f) {
			return
		}
		out, err := s.AddFault(f)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusCreated, out)
	})
	api.HandleFunc("DELETE /api/faults/{id}", func(w http.ResponseWriter, r *http.Request) {
		deleted(w)(s.DeleteFaults(r.PathValue("id"), ""))
	})
	api.HandleFunc("DELETE /api/faults", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "" {
			writeError(w, http.StatusBadRequest, errors.New("name query parameter is required"))
			return
		}
		deleted(w)(s.DeleteFaults("", name))
	})
	api.HandleFunc("GET /api/queries", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.Queries())
	})
	api.HandleFunc("POST /api/reset", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Reset(); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.Handle("/api/", requireToken(adminToken, api))
	return mux
}

func requireToken(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	want := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), want) != 1 {
			writeError(w, http.StatusUnauthorized, errors.New("missing or wrong admin token"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// resolveJSON is the Google-style JSON DoH API (https://dns.google/resolve?name=&type=), the
// shape zones-service's dnsLookup reads.
func resolveJSON(w http.ResponseWriter, r *http.Request, s *Store) {
	name := r.URL.Query().Get("name")
	qtype := uint16(dns.TypeA)
	if t := r.URL.Query().Get("type"); t != "" {
		if n, err := strconv.Atoi(t); err == nil {
			qtype = uint16(n)
		} else if v, ok := dns.StringToType[strings.ToUpper(t)]; ok {
			qtype = v
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]any{"Status": dns.RcodeFormatError, "Comment": "unknown type " + t})
			return
		}
	}
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"Status": dns.RcodeFormatError, "Comment": "name is required"})
		return
	}
	a := s.Resolve(r.Context(), "doh-json", name, qtype)
	if a.Drop {
		holdUntilTimeout(r)
		return
	}
	type jsonRR struct {
		Name string `json:"name"`
		Type uint16 `json:"type"`
		TTL  uint32 `json:"TTL"`
		Data string `json:"data"`
	}
	answers := make([]jsonRR, 0, len(a.RRs))
	for _, rr := range a.RRs {
		h := rr.Header()
		answers = append(answers, jsonRR{
			Name: h.Name, Type: h.Rrtype, TTL: h.Ttl,
			Data: strings.TrimPrefix(rr.String(), h.String()),
		})
	}
	body := map[string]any{
		"Status": a.Rcode, "TC": false, "RD": true, "RA": true, "AD": false, "CD": false,
		"Question": []map[string]any{{"name": dns.Fqdn(name), "type": qtype}},
	}
	if len(answers) > 0 {
		body["Answer"] = answers
	}
	w.Header().Set("Content-Type", "application/dns-json")
	writeJSON(w, http.StatusOK, body)
}

// dohWire is RFC 8484 DNS-over-HTTPS (wire format, GET ?dns= or POST body).
func dohWire(w http.ResponseWriter, r *http.Request, s *Store) {
	var raw []byte
	var err error
	if r.Method == http.MethodGet {
		raw, err = base64.RawURLEncoding.DecodeString(r.URL.Query().Get("dns"))
	} else {
		raw, err = io.ReadAll(io.LimitReader(r.Body, 65535))
	}
	if err != nil || len(raw) == 0 {
		http.Error(w, "malformed DNS query", http.StatusBadRequest)
		return
	}
	q := new(dns.Msg)
	if err := q.Unpack(raw); err != nil {
		http.Error(w, "malformed DNS query", http.StatusBadRequest)
		return
	}
	resp, drop := answerMsg(r.Context(), s, "doh", q)
	if drop {
		holdUntilTimeout(r)
		return
	}
	out, err := resp.Pack()
	if err != nil {
		http.Error(w, "packing answer failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/dns-message")
	_, _ = w.Write(out)
}

// holdUntilTimeout is the HTTP face of a "timeout" fault: the caller's own timeout fires first.
func holdUntilTimeout(r *http.Request) {
	select {
	case <-r.Context().Done():
	case <-time.After(timeoutHold):
	}
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("malformed JSON body: "+err.Error()))
		return false
	}
	return true
}

func deleted(w http.ResponseWriter) func(int, error) {
	return func(n int, err error) {
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{"deleted": n})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"message": err.Error()}})
}
