// HTTP surface: POST /graphql (the only real endpoint) and GET /health.
//
// Auth here is decode-only, same as the Rust gateway: a caller needs a token that verifies
// against this gateway's own configured RSA public key (the 3 WAF services' shared keypair —
// see cmd/waf-graphql-gateway/main.go's doc comment for why there's no JWKS dual-dispatch in
// this Go port) to reach any resolver at all. The SAME raw token is then forwarded verbatim to
// whichever upstream a resolver calls (gRPC metadata or an HTTP Authorization header) — that
// upstream re-verifies and does the real, tenant/role-aware authorization. Decoding locally
// first is defense in depth and a cleaner error for an obviously-bad token; it is not the
// authorization check.
package graphqlgateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/gqlerrors"

	"github.com/metap-org/metap/pkg/jwks"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/runtime"
)

type Server struct {
	schema   graphql.Schema
	verifier *jwks.TokenVerifier
	limits   Limits
}

func NewServer(schema graphql.Schema, verifier *jwks.TokenVerifier, limits Limits) *Server {
	return &Server{schema: schema, verifier: verifier, limits: limits}
}

type graphqlRequestBody struct {
	Query         string                 `json:"query"`
	OperationName string                 `json:"operationName"`
	Variables     map[string]interface{} `json:"variables"`
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) handleGraphQL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST only"})
		return
	}

	token, msg := requestToken(r)
	if token == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"errors": []map[string]string{{"message": msg}},
		})
		return
	}
	if _, err := jwks.DecodeWithVerifier(r.Context(), token, s.verifier, nil); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"errors": []map[string]string{{"message": "invalid or expired token"}},
		})
		return
	}

	malformed := func() {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"errors": []map[string]string{{"message": "malformed request body"}},
		})
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBytes))
	if err != nil {
		malformed()
		return
	}

	rc := &permission.RequestContext{ForwardedBearerToken: token}
	ctx := withRequestContext(r.Context(), rc)
	execute := func(body graphqlRequestBody) *graphql.Result {
		if err := checkLimits(body.Query, body.OperationName, s.limits); err != nil {
			return &graphql.Result{Errors: []gqlerrors.FormattedError{gqlerrors.NewFormattedError(err.Error())}}
		}
		return graphql.Do(graphql.Params{
			Schema:         s.schema,
			RequestString:  body.Query,
			VariableValues: body.Variables,
			OperationName:  body.OperationName,
			Context:        ctx,
		})
	}

	// A JSON array is a batch (@metap/platform-ui's graphqlClient.ts coalesces every query issued
	// in one microtask into one request, the same GraphQLBatchRequest shape the Rust gateway
	// accepted): run each in order, answer with an array of results in the same order.
	if trimmed := bytes.TrimLeft(raw, " \t\r\n"); len(trimmed) > 0 && trimmed[0] == '[' {
		var batch []graphqlRequestBody
		if err := json.Unmarshal(trimmed, &batch); err != nil || len(batch) == 0 {
			malformed()
			return
		}
		if len(batch) > maxBatchSize {
			writeJSON(w, http.StatusBadRequest, map[string]interface{}{
				"errors": []map[string]string{{"message": fmt.Sprintf("batch exceeds %d operations", maxBatchSize)}},
			})
			return
		}
		results := make([]*graphql.Result, len(batch))
		for i, body := range batch {
			results[i] = execute(body)
		}
		writeJSON(w, http.StatusOK, results)
		return
	}

	var body graphqlRequestBody
	if err := json.Unmarshal(raw, &body); err != nil {
		malformed()
		return
	}
	writeJSON(w, http.StatusOK, execute(body))
}

const (
	maxRequestBytes = 1 << 20 // 1 MiB — every portal request is a small query + variables
	maxBatchSize    = 50
)

// requestToken mirrors metap's httpserver authenticate: an explicit Authorization header always
// wins; otherwise the HttpOnly session cookie (what the Customer Portal's browser sends — it has
// carried no Authorization header since the cookie-session move), double-submit CSRF-checked for
// any state-changing method. /graphql is POST-only, so a cookie caller always needs the CSRF
// header. Returns "" plus an error message when neither yields a usable token.
func requestToken(r *http.Request) (string, string) {
	if header := r.Header.Get("Authorization"); header != "" {
		token, ok := runtime.ParseBearer(header)
		if !ok {
			return "", "missing or malformed Authorization header"
		}
		return token, ""
	}
	session, err := r.Cookie(runtime.SessionCookieName)
	if err != nil || session.Value == "" {
		return "", "missing or malformed Authorization header"
	}
	if runtime.RequiresCSRFCheck(r.Method) {
		csrfValue, csrfOK := "", false
		if c, cErr := r.Cookie(runtime.CSRFCookieName); cErr == nil {
			csrfValue, csrfOK = c.Value, true
		}
		csrfHeader := r.Header.Get(runtime.CSRFHeaderName)
		if !runtime.CSRFMatches(csrfValue, csrfHeader, csrfOK, csrfHeader != "") {
			return "", "missing or invalid CSRF token"
		}
	}
	return session.Value, ""
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
}

func (s *Server) Router() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/graphql", s.handleGraphQL)
	mux.HandleFunc("/health", s.handleHealth)
	return mux
}
