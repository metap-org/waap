// Layer: APP · outbound call. Delivers a notification and records it.

package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/httpserver"
	"github.com/metap-org/metap/pkg/kit/httpx"
	"github.com/metap-org/metap/pkg/kit/standard"
	"github.com/metap-org/metap/pkg/permission"
	"github.com/metap-org/waap/model"
)

func channelsObject(raw json.RawMessage) map[string]json.RawMessage {
	var obj map[string]json.RawMessage
	_ = json.Unmarshal(raw, &obj)
	return obj
}

// deliver actually delivers a notification. channels on an AlertPolicy is free-form JSON, so this
// reads it defensively: {"webhook": "https://..."} posts, {"email": "..."} logs (there is no
// mail transport in this product yet — saying so in a log line is more honest than a
// deliveryStatus: "sent" nobody can verify).
func deliver(channels json.RawMessage, payload any) (bool, string) {
	obj := channelsObject(channels)
	if raw, ok := obj["webhook"]; ok {
		if url, ok := jsonAsString(raw); ok {
			body, _ := json.Marshal(payload)
			resp, err := httpx.NewClient(deliveryTimeout).Post(url, standard.MediaTypeJSON, bytes.NewReader(body))
			if err != nil {
				return false, fmt.Sprintf("webhook error: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return true, fmt.Sprintf("webhook %d", resp.StatusCode)
			}
			return false, fmt.Sprintf("webhook %d", resp.StatusCode)
		}
	}
	if raw, ok := obj["email"]; ok {
		if address, ok := jsonAsString(raw); ok {
			return true, fmt.Sprintf("email logged to %s", address)
		}
	}
	return false, "no deliverable channel configured"
}

func channelName(channels json.RawMessage) string {
	obj := channelsObject(channels)
	if _, ok := obj["webhook"]; ok {
		return "webhook"
	}
	if _, ok := obj["email"]; ok {
		return "email"
	}
	return "none"
}

// recordNotification writes the delivery-log row every firing produces, sent or failed alike —
// an alert that failed to deliver is exactly the one an operator needs to find later.
func recordNotification(ctx context.Context, state *httpserver.AppState, rc *permission.RequestContext, policy crud.RecordDto, channel string, delivered bool) (uuid.UUID, error) {
	status := "failed"
	if delivered {
		status = "sent"
	}
	data := crud.JsonObject{
		model.FieldAlertPolicyID:  jsonStr(policy.ID.String()),
		model.FieldChannel:        jsonStr(channel),
		model.FieldDeliveryStatus: jsonStr(status),
		model.FieldTriggeredAt:    jsonStr(standard.NowRFC3339()),
	}
	result, err := state.Crud.Create(ctx, model.AlertNotifications, data, rc, nil)
	if err != nil {
		return uuid.Nil, fmt.Errorf("alert_notifications: %w", err)
	}
	return result.ID, nil
}
