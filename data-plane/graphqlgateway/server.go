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
	"encoding/json"
	"net/http"

	"github.com/graphql-go/graphql"

	"github.com/metap-org/metap/pkg/jwks"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/runtime"
)

type Server struct {
	schema   graphql.Schema
	verifier *jwks.TokenVerifier
}

func NewServer(schema graphql.Schema, verifier *jwks.TokenVerifier) *Server {
	return &Server{schema: schema, verifier: verifier}
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

	authHeader := r.Header.Get("Authorization")
	token, ok := runtime.ParseBearer(authHeader)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"errors": []map[string]string{{"message": "missing or malformed Authorization header"}},
		})
		return
	}
	if _, err := jwks.DecodeWithVerifier(r.Context(), token, s.verifier, nil); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"errors": []map[string]string{{"message": "invalid or expired token"}},
		})
		return
	}

	var body graphqlRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"errors": []map[string]string{{"message": "malformed request body"}},
		})
		return
	}

	rc := &permission.RequestContext{ForwardedBearerToken: token}
	ctx := withRequestContext(r.Context(), rc)

	result := graphql.Do(graphql.Params{
		Schema:         s.schema,
		RequestString:  body.Query,
		VariableValues: body.Variables,
		OperationName:  body.OperationName,
		Context:        ctx,
	})
	writeJSON(w, http.StatusOK, result)
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
