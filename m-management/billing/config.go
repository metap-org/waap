// Layer: APP · wiring. Reads configuration and plugs the app's pieces into metap's app.Options; no business rules.

package billing

import (
	"github.com/google/uuid"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/kit/env"
)

// Config is what billing-service reads from its environment, once, in cmd/ (code standard 2.10).
type Config struct {
	// OperatorTenant owns the plan rows; the zero value means no plans are enforced.
	OperatorTenant uuid.UUID
}

// LoadConfig reads OPERATOR_TENANT_ID (environment, then the config file).
func LoadConfig() Config {
	id, _ := uuid.Parse(env.String("OPERATOR_TENANT_ID", ""))
	return Config{OperatorTenant: id}
}

// PlanSource reads plans through backend; it is what the zones domain asks for a tenant's limits.
func (c Config) PlanSource(backend crud.RecordBackend) *PlanSource {
	return &PlanSource{Operator: c.OperatorTenant, Backend: backend}
}
