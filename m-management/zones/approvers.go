// Layer: APP · wiring. Counts who could approve a policy change, from the tenant's members and roles.

package zones

import (
	"context"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/auth/identity"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/permission/rolestore"
	"github.com/metap-org/metap/pkg/web"
	"github.com/metap-org/waap/m-management/zones/guard"
	"github.com/metap-org/waap/model"
)

// Approvers counts the active people (not service accounts) who may change firewall rules: through a
// tenant-wide role, or a role held in the account in question.
func Approvers(state *web.State) guard.ApproverCounter { return approvers{state: state} }

type approvers struct{ state *web.State }

func (a approvers) CountApprovers(ctx context.Context, accountID uuid.UUID, rc *permission.RequestContext) (int, error) {
	tenant, err := uuid.Parse(rc.TenantID)
	if err != nil {
		return 0, err
	}
	pool, err := a.state.Router.PoolFor(ctx, tenant)
	if err != nil {
		return 0, err
	}
	assigned, err := rolestore.ListUsers(ctx, pool, tenant)
	if err != nil {
		return 0, err
	}
	roles := map[uuid.UUID][]string{}
	for _, user := range assigned {
		roles[user.UserID] = append(roles[user.UserID], user.Roles...)
	}
	if accountID != uuid.Nil {
		scoped, err := rolestore.MembersOfScope(ctx, pool, tenant, model.ScopeAccount, accountID.String())
		if err != nil {
			return 0, err
		}
		for _, member := range scoped {
			roles[member.UserID] = append(roles[member.UserID], member.Role)
		}
	}

	var may []uuid.UUID
	for user, held := range roles {
		as := &permission.RequestContext{TenantID: rc.TenantID, UserID: user.String(), Roles: held, HasRoles: true}
		decision, err := a.state.Permissions.CanUpdateEntity(ctx, as, model.FirewallRules)
		if err != nil {
			return 0, err
		}
		if decision.Allowed {
			may = append(may, user)
		}
	}
	if len(may) == 0 {
		return 0, nil
	}
	_, total, err := identity.ListMembers(ctx, a.state.Router.SharedPool(), tenant, identity.ListQuery{IDs: may, Status: "active", Limit: 1})
	return total, err
}
