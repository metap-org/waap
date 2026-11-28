package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/kit/standard"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/metap/pkg/query"
	"github.com/metap-org/waap/model"
)

// Backend serves the read model as a crud.RecordBackend. Writes are refused: events arrive only
// through Analytics' own ingest.
//
// A read signs a short-lived token for what the caller may see (see ReadTokens) and sends that, not
// the caller's own token: the records live outside metap, so the Analytics API enforces the grant
// itself from the signature.
type Backend struct {
	Client *Client
	Tokens *ReadTokens
}

var _ crud.RecordBackend = (*Backend)(nil)

// ToRecord renders an event in the generic record envelope the portal reads.
func ToRecord(e Event) crud.RecordDto {
	str := func(s string) json.RawMessage { raw, _ := json.Marshal(s); return raw }
	data := crud.JsonObject{
		model.FieldZoneID:        str(e.ZoneID),
		model.FieldTriggeredBy:   str(e.TriggeredBy),
		model.FieldTriggeredByID: str(e.TriggeredByID),
		model.FieldAction:        str(e.Action),
		model.FieldSourceIP:      str(e.SourceIP),
		model.FieldRequestPath:   str(e.RequestPath),
		model.FieldOccurredAt:    str(e.OccurredAt.UTC().Format(standard.TimestampMillis)),
	}
	if e.TriggeredByName != "" {
		data[model.FieldTriggeredByName] = str(e.TriggeredByName)
	}
	return crud.RecordDto{
		ID: e.ID, Entity: SecurityEvents, Data: data, Version: 1,
		CreatedAt: e.IngestedAt, UpdatedAt: e.IngestedAt,
	}
}

// failure maps an Analytics API error onto a ServiceError, keeping its status and code; any
// other error (network, decode) stays a plain internal error.
func failure(err error) error {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return crud.NewServiceErrorMsg(uint16(apiErr.Status), apiErr.Code, apiErr.Message)
	}
	return err
}

func readOnly() error {
	return crud.NewServiceErrorMsg(http.StatusMethodNotAllowed, "read_only",
		"Security events are written by the edge through the Analytics plane, not through this API.")
}

func (b *Backend) List(ctx context.Context, _ string, input query.ListInput, rc *permission.RequestContext) (crud.ListResult, error) {
	tok, err := b.Tokens.ForCaller(ctx, rc)
	if err != nil {
		return crud.ListResult{}, err
	}
	if input.JQL != nil && strings.TrimSpace(*input.JQL) != "" {
		return crud.ListResult{}, crud.NewServiceErrorMsg(http.StatusBadRequest, "invalid_query", "jql is not supported for security events; use filter.")
	}
	p := ListParams{Filters: map[string]string{}, Limit: int(input.Limit)}
	for _, f := range input.Filters {
		if f[1] != "" {
			p.Filters[f[0]] = f[1]
		}
	}
	if input.Sort != nil && *input.Sort == "occurredAt" {
		p.Ascending = true
	}
	if input.Cursor != nil {
		p.Cursor = *input.Cursor
	}
	page, err := b.Client.List(ctx, tok, p)
	if err != nil {
		return crud.ListResult{}, failure(err)
	}
	out := make([]crud.RecordDto, 0, len(page.Records))
	for _, e := range page.Records {
		out = append(out, ToRecord(e))
	}
	return crud.ListResult{Data: out, Page: crud.PageInfo{Limit: int64(p.Limit), NextCursor: page.NextCursor}}, nil
}

func (b *Backend) Get(ctx context.Context, _ string, id uuid.UUID, rc *permission.RequestContext) (crud.GetResult, error) {
	tok, err := b.Tokens.ForCaller(ctx, rc)
	if err != nil {
		return crud.GetResult{}, err
	}
	e, err := b.Client.Get(ctx, tok, id)
	if err != nil {
		return crud.GetResult{}, failure(err)
	}
	return crud.GetResult{Record: ToRecord(e)}, nil
}

func (b *Backend) GetMany(ctx context.Context, entity string, ids []uuid.UUID, rc *permission.RequestContext) ([]crud.GetManyResult, error) {
	out := make([]crud.GetManyResult, 0, len(ids))
	for _, id := range ids {
		r, err := b.Get(ctx, entity, id, rc)
		if err != nil {
			if _, business := crud.AsServiceError(err); business {
				continue // a missing or denied id is simply absent from the batch
			}
			return nil, err
		}
		out = append(out, crud.GetManyResult{ID: id, Record: r.Record})
	}
	return out, nil
}

func (b *Backend) Create(context.Context, string, crud.JsonObject, *permission.RequestContext, *string) (crud.RecordDto, error) {
	return crud.RecordDto{}, readOnly()
}

func (b *Backend) Update(context.Context, string, uuid.UUID, int32, crud.JsonObject, *permission.RequestContext, *string) (crud.RecordDto, error) {
	return crud.RecordDto{}, readOnly()
}

func (b *Backend) Transition(context.Context, string, uuid.UUID, string, int32, crud.JsonObject, *permission.RequestContext, *string) (crud.RecordDto, error) {
	return crud.RecordDto{}, readOnly()
}

func (b *Backend) Delete(context.Context, string, uuid.UUID, int32, *permission.RequestContext, *string) (crud.RecordDto, error) {
	return crud.RecordDto{}, readOnly()
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (b *Backend) Aggregate(ctx context.Context, _ string, spec query.AggregateSpec, rc *permission.RequestContext) ([]json.RawMessage, error) {
	tok, err := b.Tokens.ForCaller(ctx, rc)
	if err != nil {
		return nil, err
	}
	req := AggregateSpec{
		Metrics: spec.Metrics, GroupBy: deref(spec.GroupBy), Bucket: deref(spec.Bucket),
		TimeField: deref(spec.TimeField), Filters: spec.Filters, Since: deref(spec.Since), Until: deref(spec.Until),
	}
	if spec.Limit != nil {
		req.Limit = int(*spec.Limit)
	}
	rows, err := b.Client.Aggregate(ctx, tok, req)
	if err != nil {
		return nil, failure(err)
	}
	return rows, nil
}

// Window lists events since a cutoff, newest first, up to limit — what the alerting
// domain's correlation and alert evaluation read. The time bound is pushed to Analytics rather than
// filtered after the fact, so a busy zone can't push in-window events past the limit with
// out-of-window ones.
func (c *Client) Window(ctx context.Context, token string, filters map[string]string, since time.Time, limit int) ([]crud.RecordDto, error) {
	page, err := c.List(ctx, token, ListParams{Filters: filters, Since: &since, Limit: limit})
	if err != nil {
		return nil, err
	}
	out := make([]crud.RecordDto, 0, len(page.Records))
	for _, e := range page.Records {
		out = append(out, ToRecord(e))
	}
	return out, nil
}
