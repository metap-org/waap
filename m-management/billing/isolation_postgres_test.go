//go:build integration

package billing_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/waap/m-management/billing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The operator holds every customer's subscription and request. What is read for one customer must
// be that customer's: the lookups filter on customerTenantId.
func TestOneCustomerNeverSeesOrGetsAnothers(t *testing.T) {
	pool := testPool(t)
	registry := setupRegistryAll(t, pool)
	operator, userID := uuid.New(), uuid.New()
	seedAdmin(t, pool, operator, userID)
	state, _ := newTestState(t, pool, registry)
	ctx := context.Background()
	opRC := adminContext(operator, userID)
	src := &billing.PlanSource{Operator: operator, Backend: state.Crud}

	mk := func(name string, zones int, def bool) crud.RecordDto {
		r, err := state.Crud.Create(ctx, "waf.plans", obj(t, map[string]any{"name": name, "maxZones": zones, "maxRulesPerZone": zones * 5, "isDefault": def, "enabled": true}), opRC, nil)
		require.NoError(t, err)
		return r
	}
	_, big, mid := mk("Small", 1, true), mk("Big", 10, false), mk("Mid", 5, false)
	acme, globex := uuid.New(), uuid.New()
	_, err := state.Crud.Create(ctx, "waf.subscriptions", obj(t, map[string]any{"customerTenantId": acme.String(), "planId": big.ID.String()}), opRC, nil)
	require.NoError(t, err)

	planOf := func(tenant uuid.UUID) string {
		r, err := src.Resolve(ctx, tenant)
		require.NoError(t, err)
		return r.Plan.Name
	}
	assert.Equal(t, "Big", planOf(acme))
	assert.Equal(t, "Small", planOf(globex), "a customer without a subscription gets the default plan")

	acmeRC, globexRC := adminContext(acme, userID), adminContext(globex, userID)
	_, err = src.RequestPlanChange(ctx, globexRC, mid.ID.String(), "globex private note", "Globex")
	require.NoError(t, err)
	seen, err := src.MyPlanRequests(ctx, acmeRC)
	require.NoError(t, err)
	assert.Empty(t, seen, "another customer's request is not mine")
	mine, err := src.MyPlanRequests(ctx, globexRC)
	require.NoError(t, err)
	assert.Len(t, mine, 1)
	// And one customer's waiting request does not stop another from asking.
	_, err = src.RequestPlanChange(ctx, acmeRC, mid.ID.String(), "", "Acme")
	assert.NoError(t, err)
}
