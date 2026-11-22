// Layer: APP · declared-action logic. A business function behind a declared action; the zones package wires it (actions.go).

// Exporting a tenant's configuration (UC-80): everything this service holds for it, as one JSON
// document the customer keeps. Read as the caller, so it contains what they may see; secrets never
// are in it (the verification token of a domain is the public TXT value, not a secret).
package policy

import (
	"context"
	"time"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"
	"github.com/metap-org/waap/m-management/zones/versions"
	"github.com/metap-org/waap/model"
)

// exportedEntities are the entities in an export, in the order a reader meets them.
var exportedEntities = []string{
	model.Accounts, model.Domains, model.Zones, model.DdosPolicies, model.FirewallRules, model.IPAccessLists,
}

// exportRecord is one record of an export: id and the values, nothing internal.
type exportRecord struct {
	ID   string          `json:"id"`
	Data crud.JsonObject `json:"data"`
}

// maxExportRecords bounds one entity in one export, so a runaway tenant cannot make one request
// read the table; hitting it says so (Truncated) instead of silently cutting the file.
const maxExportRecords = 20000

// ExportDocument is the file the customer downloads.
type ExportDocument struct {
	ExportedAt string                    `json:"exportedAt"`
	TenantID   string                    `json:"tenantId"`
	Entities   map[string][]exportRecord `json:"entities"`
	Truncated  []string                  `json:"truncated"`
}

// Export reads every exported entity in full, a page at a time.
func Export(ctx context.Context, backend crud.RecordBackend, rc *permission.RequestContext) (ExportDocument, error) {
	doc := ExportDocument{
		ExportedAt: time.Now().UTC().Format(time.RFC3339), TenantID: rc.TenantID,
		Entities: map[string][]exportRecord{}, Truncated: []string{},
	}
	for _, entity := range exportedEntities {
		records := []exportRecord{}
		for offset := int64(0); ; offset += versions.SnapshotPage {
			page, err := backend.List(ctx, entity, query.ListInput{Limit: versions.SnapshotPage, Offset: offset}, rc)
			if err != nil {
				return ExportDocument{}, err
			}
			for _, r := range page.Data {
				records = append(records, exportRecord{ID: r.ID.String(), Data: r.Data})
			}
			if int64(len(page.Data)) < versions.SnapshotPage {
				break
			}
			if int64(len(records)) >= maxExportRecords {
				doc.Truncated = append(doc.Truncated, entity)
				break
			}
		}
		doc.Entities[entity] = records
	}
	return doc, nil
}
