package zonesservice

import (
	"context"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/permission"
)

// TenantUsageForTest exposes the usage view to the external integration tests.
func TenantUsageForTest(ctx context.Context, backend crud.RecordBackend, plans *PlanClient, rc *permission.RequestContext) (usageView, error) {
	return tenantUsage(ctx, backend, plans, rc)
}
