// Package analytics is control-plane's writer into the Analytics plane: edge telemetry goes up
// as batches to analytics-api's ingest endpoint (ADR-0001: telemetry belongs to Analytics, not to
// Management's database).
//
// Auth is the same service-account token this process already holds for Management (the tenant
// the events are filed under is that token's), plus the deployment's ingest secret, so a portal
// user's token alone can't write events. Until per-zone tenant routing exists, every event lands
// in that one service account's tenant — the same behaviour the old Management write path had.
package analytics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/metap-org/metap/pkg/runtime"
)

// IngestHeader carries the ingest secret (analytics-plane/api.IngestHeader).
const IngestHeader = "X-Analytics-Ingest-Token"

type Client struct {
	url         string
	ingestToken string
	http        *http.Client
	token       *runtime.ServiceTokenSource
}

func New(httpClient *http.Client, baseURL, ingestToken string, token *runtime.ServiceTokenSource) *Client {
	return &Client{url: strings.TrimSuffix(baseURL, "/") + "/v1/security-events/ingest", ingestToken: ingestToken, http: httpClient, token: token}
}

// WriteSecurityEvents sends one batch. Events the API rejects as invalid are reported back and
// dropped (retrying them can't succeed); a transport or 5xx failure returns an error for the
// whole batch.
func (c *Client) WriteSecurityEvents(ctx context.Context, events []map[string]any) (rejected int, err error) {
	body, err := json.Marshal(map[string]any{"events": events})
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token.Current())
	if c.ingestToken != "" {
		req.Header.Set(IngestHeader, c.ingestToken)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("POST %s: %w", c.url, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusAccepted {
		return 0, fmt.Errorf("POST %s returned %d: %s", c.url, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out struct {
		Data struct {
			Rejected []json.RawMessage `json:"rejected"`
		} `json:"data"`
	}
	_ = json.Unmarshal(raw, &out)
	return len(out.Data.Rejected), nil
}
