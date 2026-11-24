// Layer: APP · helper. Reading Analytics events and records for the sweeps.

package alerting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"
	"github.com/metap-org/metap/pkg/web"
	"github.com/metap-org/waap/m-management/analytics"
)

// recentEvents reads events since cutoff (newest first, at most eventScanLimit). An Analytics
// error keeps its own status (401 stays 401, 503 stays 503) instead of turning into a generic 500.
func recentEvents(ctx context.Context, cfg Config, state *web.State, rc *permission.RequestContext, filters map[string]string, cutoff time.Time) ([]crud.RecordDto, error) {
	tokens := &analytics.ReadTokens{Signer: state.TokenSigner}
	token, err := tokens.TenantWide(rc)
	if err != nil {
		return nil, err
	}
	events, err := cfg.analytics().Window(ctx, token, filters, cutoff, eventScanLimit)
	if err != nil {
		var apiErr *analytics.APIError
		if errors.As(err, &apiErr) {
			return nil, crud.NewServiceErrorMsg(uint16(apiErr.Status), apiErr.Code, apiErr.Message)
		}
		return nil, err
	}
	return events, nil
}

func jsonStr(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

func jsonAsString(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

func jsonAsInt(raw json.RawMessage) (int64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var n float64
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, false
	}
	return int64(n), true
}

func strField(record crud.RecordDto, field string) string {
	s, _ := jsonAsString(record.Data[field])
	return s
}

// listRecords lists up to limit records with plain equality filters, unwrapping the service
// result into a plain error — every caller here treats a permission/validation failure as "abort
// the sweep", not as an empty set, because silently proceeding on an empty read would create
// nothing and look exactly like "there was nothing to do".
func listRecords(ctx context.Context, state *web.State, entity string, filters [][2]string, limit int64, rc *permission.RequestContext) ([]crud.RecordDto, error) {
	result, err := state.Crud.List(ctx, entity, query.ListInput{Limit: limit, Filters: filters}, rc)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", entity, err)
	}
	return result.Data, nil
}
