// SecurityEvent retention cleanup — the one piece of the decided-but-unbuilt v1 retention policy
// (data-plane/docs/02-domain-model.md's "Chốt tiếp (2026-08-30)" section): "30 ngày mặc định...
// Không build cơ chế archive S3 thật ở v1 — hết hạn thì xoá (dùng metap-cron Schedule job dọn
// định kỳ)". The Plan-tier override (`accessLogRetentionDays`-style, per
// data-plane/docs/12-billing-plans.md) stays undone along with the rest of billing/Plan, which
// has no entity or code anywhere in this product yet — this ships the flat 30-day default the
// doc already names as the fallback, not a per-tenant variant of something that doesn't exist.
package alertingservice

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/metap-org/metap/pkg/httpserver"
	"github.com/metap-org/metap/pkg/reconciler"
)

// DefaultSecurityEventRetentionDays is the settled v1 default from the domain-model doc above.
const DefaultSecurityEventRetentionDays = 30

// securityEventDeleteBatchSize mirrors metap-go's own oauthserver.DeleteBatchSize convention —
// waf.security_events is explicitly the highest-volume entity in this product (see
// entities/securityevent.go's own doc comment), so a first sweep after this ever runs (or after
// a long gap in the schedule) can face a large backlog; one unbounded DELETE over that would
// hold its row locks and bloat WAL for the whole statement.
const securityEventDeleteBatchSize = 5_000

// securityEventsTable is the real, schema-qualified physical table — same helper the entity
// definition itself uses, so this can't drift from what CrudService actually reads/writes.
var securityEventsTable = reconciler.QualifiedTableNameIn("waf.security_events", "waf")

// deleteExpiredSecurityEvents runs one batched sweep against the shared pool directly rather
// than through CrudService/Router: this is a genuinely cross-tenant maintenance delete (every
// tenant's rows older than the cutoff, in one shared Schema-strategy table), not a read or write
// scoped to one tenant's RequestContext — same reasoning as metap-go's own oauth_cleanup sweep.
// SKIP LOCKED lets a concurrent sweep (an operator-triggered on-demand run overlapping the
// scheduled one) split the work instead of blocking on the other's row locks. Every row here has
// a uuid primary key id, same as every other dedicated-table entity.
func deleteExpiredSecurityEvents(ctx context.Context, pool *pgxpool.Pool, retentionDays int) (int64, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	// occurredAt has no real physical column — EntityField.Indexed/.Sortable alone only ever
	// promote a field to an *expression* index straight over `data` (metap-go's own
	// FieldHasRealColumn doc comment: a real column needs a Reference or an explicit
	// Storage: Column override, neither of which this field has). Confirmed live against this
	// exact table, not assumed — the plain `"occurredAt"` column reference this file's first
	// version used failed with "column does not exist" the moment the e2e test below ran it.
	sql := `DELETE FROM ` + securityEventsTable + ` WHERE id IN (
		SELECT id FROM ` + securityEventsTable + ` WHERE (data ->> 'occurredAt')::timestamptz < $1
		LIMIT $2 FOR UPDATE SKIP LOCKED
	)`
	var total int64
	for {
		tag, err := pool.Exec(ctx, sql, cutoff, securityEventDeleteBatchSize)
		if err != nil {
			return total, err
		}
		deleted := tag.RowsAffected()
		total += deleted
		if deleted < securityEventDeleteBatchSize {
			return total, nil
		}
	}
}

type cleanupSecurityEventsBody struct {
	RetentionDays *int `json:"retentionDays"`
}

// cleanupSecurityEvents is POST /internal/security-events/cleanup — the scheduled sweep,
// following the exact same "endpoint, not a background loop, so metap-cron can drive it" shape
// this file's own doc comment already established for correlateIncidents/evaluateAlerts.
func cleanupSecurityEvents(w http.ResponseWriter, r *http.Request, state *httpserver.AppState) {
	_, ok := httpserver.RequireAuth(w, r, state)
	if !ok {
		return
	}

	var body cleanupSecurityEventsBody
	if r.ContentLength != 0 {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	retentionDays := DefaultSecurityEventRetentionDays
	if body.RetentionDays != nil && *body.RetentionDays > 0 {
		retentionDays = *body.RetentionDays
	}

	deleted, err := deleteExpiredSecurityEvents(r.Context(), state.Pool, retentionDays)
	if err != nil {
		httpserver.WriteInternalError(w, err)
		return
	}

	httpserver.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"deletedCount":  deleted,
		"retentionDays": retentionDays,
	}})
}
