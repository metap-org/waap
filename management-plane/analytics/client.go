package analytics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Event mirrors the Analytics query API's event shape.
type Event struct {
	ID              uuid.UUID `json:"id"`
	ZoneID          string    `json:"zoneId"`
	TriggeredBy     string    `json:"triggeredBy"`
	TriggeredByID   string    `json:"triggeredById"`
	TriggeredByName string    `json:"triggeredByName"`
	Action          string    `json:"action"`
	SourceIP        string    `json:"sourceIp"`
	RequestPath     string    `json:"requestPath"`
	OccurredAt      time.Time `json:"occurredAt"`
	IngestedAt      time.Time `json:"ingestedAt"`
}

type Page struct {
	Records    []Event `json:"records"`
	NextCursor *string `json:"nextCursor"`
	HasMore    bool    `json:"hasMore"`
}

// ListParams: Filters are equality filters on the API's allowlisted fields.
type ListParams struct {
	Filters   map[string]string
	Since     *time.Time
	Until     *time.Time
	Ascending bool
	Limit     int
	Cursor    string
}

// APIError is a non-2xx answer from Analytics, kept with its status and code so callers can pass
// it through (a 400 stays a 400, a 401 a 401).
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("analytics: %d %s: %s", e.Status, e.Code, e.Message)
}

type Client struct {
	base string
	http *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{base: strings.TrimSuffix(baseURL, "/"), http: &http.Client{Timeout: 15 * time.Second}}
}

func (c *Client) do(ctx context.Context, token, method, path string, body any, out any) error {
	if token == "" {
		return &APIError{Status: http.StatusUnauthorized, Code: "unauthorized", Message: "no caller token to forward to Analytics"}
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return &APIError{Status: http.StatusServiceUnavailable, Code: "analytics_unavailable", Message: "Analytics is unreachable"}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		var e struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Error.Code == "" {
			e.Error.Code, e.Error.Message = "analytics_error", http.StatusText(resp.StatusCode)
		}
		return &APIError{Status: resp.StatusCode, Code: e.Error.Code, Message: e.Error.Message}
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("analytics: malformed response: %w", err)
	}
	return json.Unmarshal(envelope.Data, out)
}

func (c *Client) List(ctx context.Context, token string, p ListParams) (Page, error) {
	q := url.Values{}
	for k, v := range p.Filters {
		q.Set(k, v)
	}
	if p.Since != nil {
		q.Set("since", p.Since.UTC().Format(time.RFC3339Nano))
	}
	if p.Until != nil {
		q.Set("until", p.Until.UTC().Format(time.RFC3339Nano))
	}
	if p.Ascending {
		q.Set("sort", "occurredAt")
	}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	if p.Cursor != "" {
		q.Set("cursor", p.Cursor)
	}
	var page Page
	err := c.do(ctx, token, http.MethodGet, "/v1/security-events?"+q.Encode(), nil, &page)
	return page, err
}

func (c *Client) Get(ctx context.Context, token string, id uuid.UUID) (Event, error) {
	var e Event
	err := c.do(ctx, token, http.MethodGet, "/v1/security-events/"+id.String(), nil, &e)
	return e, err
}

// AggregateSpec is the Analytics aggregate request — the same wire shape as metap's.
type AggregateSpec struct {
	Metrics   []string          `json:"metrics,omitempty"`
	GroupBy   string            `json:"groupBy,omitempty"`
	Bucket    string            `json:"bucket,omitempty"`
	TimeField string            `json:"timeField,omitempty"`
	Filters   map[string]string `json:"filters,omitempty"`
	Since     string            `json:"since,omitempty"`
	Until     string            `json:"until,omitempty"`
	Limit     int               `json:"limit,omitempty"`
}

func (c *Client) Aggregate(ctx context.Context, token string, spec AggregateSpec) ([]json.RawMessage, error) {
	var rows []json.RawMessage
	err := c.do(ctx, token, http.MethodPost, "/v1/security-events/aggregate", spec, &rows)
	return rows, err
}
