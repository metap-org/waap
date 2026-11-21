package management

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/metap-org/metap/pkg/crud"
)

func TestRecordCarriesTheLoggedInTenant(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{"tenantId": "8a9db97b-6f75-4386-98f2-f23d21f4edf8"})
	token := "h." + base64.RawURLEncoding.EncodeToString(payload) + ".s"
	assert.Equal(t, "8a9db97b-6f75-4386-98f2-f23d21f4edf8", tenantFromToken(token))
	assert.Empty(t, tenantFromToken("not-a-jwt"))
	assert.Empty(t, tenantFromToken("a.!!!.c"))

	rec := recordFrom(crud.RecordDto{ID: uuid.New(), Data: crud.JsonObject{"hostname": json.RawMessage(`"h"`)}}, []string{"hostname"}, "t1")
	tenant, ok := rec.Str("tenantId")
	assert.True(t, ok)
	assert.Equal(t, "t1", tenant)
	_, ok = recordFrom(crud.RecordDto{ID: uuid.New()}, nil, "").Str("tenantId")
	assert.False(t, ok, "no tenant known: no entry rather than an empty one")
}
