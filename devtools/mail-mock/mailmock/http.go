package mailmock

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"strings"
)

//go:embed ui.html
var uiHTML []byte

// Handler serves the UI (/) and the API:
//
//	GET    /api/messages          newest first; ?to=<address> filters by recipient
//	GET    /api/messages/latest   the newest one (?to= as above), 404 when there is none
//	GET    /api/messages/{id}     one message
//	DELETE /api/messages          drop every message
//	GET    /health
func Handler(store *Store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(uiHTML)
	})
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/messages", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"data": store.List(strings.TrimSpace(r.URL.Query().Get("to")))})
	})
	mux.HandleFunc("GET /api/messages/latest", func(w http.ResponseWriter, r *http.Request) {
		list := store.List(strings.TrimSpace(r.URL.Query().Get("to")))
		if len(list) == 0 {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": map[string]string{"code": "not_found", "message": "No message yet."}})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": list[0]})
	})
	mux.HandleFunc("GET /api/messages/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(r.PathValue("id"))
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "invalid_id", "message": "id must be a positive integer"}})
			return
		}
		m, found := store.Get(id)
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": map[string]string{"code": "not_found", "message": "No such message."}})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": m})
	})
	mux.HandleFunc("DELETE /api/messages", func(w http.ResponseWriter, _ *http.Request) {
		store.Clear()
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
