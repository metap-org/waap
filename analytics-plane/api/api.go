// Package api is the Analytics plane's HTTP surface: batch ingest (control-plane writes here) and
// the query API (Management reads here, on behalf of a portal user). Errors use the platform's
// {"error":{"code","message"}} shape so Management can pass them through unchanged.
//
// Every route but /health needs a verified platform token; the tenant comes from that token.
// Ingest additionally needs the deployment's ingest secret when one is configured: a portal
// user's token alone must not be able to forge events for its own tenant.
package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/metap-org/waap/analytics-plane/auth"
	"github.com/metap-org/waap/analytics-plane/events"
	"github.com/metap-org/waap/analytics-plane/store"
)

const (
	MaxIngestEvents = 1000
	maxBodyBytes    = 4 << 20
	IngestHeader    = "X-Analytics-Ingest-Token"
)

type Server struct {
	Store       store.Store
	Verifier    auth.Verifier
	IngestToken string // empty = no ingest secret required (dev only)
	Now         func() time.Time
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)

	authed := http.NewServeMux()
	authed.HandleFunc("POST /v1/security-events/ingest", s.ingest)
	authed.HandleFunc("GET /v1/security-events", s.list)
	authed.HandleFunc("GET /v1/security-events/{id}", s.get)
	authed.HandleFunc("POST /v1/security-events/aggregate", s.aggregate)
	mux.Handle("/v1/", auth.Middleware(s.Verifier, writeError)(authed))
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": msg}})
}

// fail maps a store/planner error to a response; anything unexpected is logged, not echoed.
func fail(w http.ResponseWriter, err error) {
	var inv *events.InvalidQueryError
	switch {
	case errors.As(err, &inv):
		writeError(w, http.StatusBadRequest, "invalid_query", inv.Message)
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Security event not found.")
	default:
		slog.Error("analytics query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Internal error.")
	}
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.Ping(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "degraded", "checks": map[string]bool{"clickhouse": false}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "checks": map[string]bool{"clickhouse": true}})
}

type rejection struct {
	Index int    `json:"index"`
	Error string `json:"error"`
}

func (s *Server) ingest(w http.ResponseWriter, r *http.Request) {
	if s.IngestToken != "" &&
		subtle.ConstantTimeCompare([]byte(r.Header.Get(IngestHeader)), []byte(s.IngestToken)) != 1 {
		writeError(w, http.StatusForbidden, "forbidden", "missing or wrong ingest token")
		return
	}
	id, _ := auth.FromContext(r.Context())
	var body struct {
		Events []events.Incoming `json:"events"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "malformed_body", "body must be {\"events\": [...]}")
		return
	}
	if len(body.Events) > MaxIngestEvents {
		writeError(w, http.StatusRequestEntityTooLarge, "too_many_events", "at most "+strconv.Itoa(MaxIngestEvents)+" events per request")
		return
	}
	now := s.now()
	valid := make([]events.Event, 0, len(body.Events))
	rejected := []rejection{}
	for i, in := range body.Events {
		e, err := in.Validate(now)
		if err != nil {
			rejected = append(rejected, rejection{Index: i, Error: err.Error()})
			continue
		}
		valid = append(valid, e)
	}
	if err := s.Store.Insert(r.Context(), id.TenantID, valid); err != nil {
		slog.Error("inserting security events", "error", err, "count", len(valid))
		writeError(w, http.StatusServiceUnavailable, "store_unavailable", "could not store events; retry")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"data": map[string]any{"accepted": len(valid), "rejected": rejected}})
}

// listParams: every allowlisted field is an equality filter; since/until/sort/limit/cursor are
// the rest. An unknown parameter is an error rather than silently ignored.
func listQuery(r *http.Request) (events.ListQuery, error) {
	q := events.ListQuery{Filters: map[string]string{}}
	values := r.URL.Query()
	var since, until string
	for key, vals := range values {
		v := vals[0]
		switch key {
		case "since":
			since = v
		case "until":
			until = v
		case "limit":
			n, err := strconv.Atoi(v)
			if err != nil {
				return q, &events.InvalidQueryError{Message: "limit must be a number"}
			}
			q.Limit = n
		case "cursor":
			q.Cursor = v
		case "sort":
			switch v {
			case "occurredAt":
				q.Ascending = true
			case "-occurredAt", "":
			default:
				return q, &events.InvalidQueryError{Message: "sort must be occurredAt or -occurredAt"}
			}
		default:
			q.Filters[key] = v
		}
	}
	tr, err := events.ParseTimeRange(since, until)
	if err != nil {
		return q, err
	}
	q.Range = tr
	return q, nil
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	q, err := listQuery(r)
	if err != nil {
		fail(w, err)
		return
	}
	page, err := s.Store.List(r.Context(), id.TenantID, q)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": page})
}

func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	eventID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Security event not found.")
		return
	}
	e, err := s.Store.Get(r.Context(), id.TenantID, eventID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": e})
}

func (s *Server) aggregate(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	var q events.AggregateQuery
	dec := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&q); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_query", "malformed aggregate spec: "+strings.TrimPrefix(err.Error(), "json: "))
		return
	}
	rows, err := s.Store.Aggregate(r.Context(), id.TenantID, q)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}
