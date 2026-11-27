// Layer: APP · guard. Gives a new zone its Domain.

package guard

import (
	"context"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"
	"github.com/metap-org/waap/m-management/zones/internal/jx"
	"github.com/metap-org/waap/m-management/zones/rules"
	"github.com/metap-org/waap/model"
)

// ---- domain ------------------------------------------------------------------------------------

// domainGuard gives a new zone its Domain (found or created from the hostname's apex). domainId is
// create-only: a zone's domain never changes once assigned.
type domainGuard struct{ passthrough }

func (g *domainGuard) Create(ctx context.Context, entity string, data crud.JsonObject, rc *permission.RequestContext, reason *string) (crud.RecordDto, error) {
	if entity != model.Zones || !hasHostname(data) {
		return g.inner.Create(ctx, entity, data, rc, reason)
	}
	hostname, _ := jx.AsString(data[model.FieldHostname])
	domainID, verified, err := g.resolveDomainID(ctx, hostname, rc)
	if err != nil {
		return crud.RecordDto{}, err
	}
	withDomain := jx.Clone(data)
	withDomain[model.FieldDomainID] = jx.String(domainID.String())
	if verified {
		withDomain[model.FieldVerificationStatus] = jx.String(model.VerificationVerified)
	}
	return g.inner.Create(ctx, entity, withDomain, rc, reason)
}

// resolveDomainID looks up (or lazily creates) the Domain row for hostname's apex and returns its
// id, plus whether that domain is already verified (a new zone under it must start verified too:
// the verify action only cascades onto zones that exist at the time).
func (g *domainGuard) resolveDomainID(ctx context.Context, hostname string, rc *permission.RequestContext) (uuid.UUID, bool, error) {
	apex := rules.ApexDomain(hostname)
	existing, err := g.inner.List(ctx, model.Domains, query.ListInput{Limit: jx.ExistenceLimit, Filters: [][2]string{{model.FieldApexDomain, apex}}}, rc)
	if err != nil {
		return uuid.Nil, false, err
	}
	if len(existing.Data) > 0 {
		status, _ := jx.AsString(existing.Data[0].Data[model.FieldVerificationStatus])
		return existing.Data[0].ID, status == model.VerificationVerified, nil
	}
	created, err := g.inner.Create(ctx, model.Domains, crud.JsonObject{
		model.FieldApexDomain:         jx.String(apex),
		model.FieldVerificationToken:  jx.String(newVerificationToken()),
		model.FieldVerificationMethod: jx.String(model.VerificationMethodDNSTxt),
		model.FieldVerificationStatus: jx.String(model.VerificationUnverified),
	}, rc, nil)
	if err != nil {
		return uuid.Nil, false, err
	}
	return created.ID, false, nil
}
