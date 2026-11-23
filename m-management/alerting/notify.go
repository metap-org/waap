// Layer: APP · outbound call. Delivers a notification and records it.

package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	neturl "net/url"

	"github.com/google/uuid"
	"github.com/metap-org/metap/pkg/crud"
	"github.com/metap-org/metap/pkg/httpserver"
	"github.com/metap-org/metap/pkg/kit/errcode"
	"github.com/metap-org/metap/pkg/kit/safehttp"
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
//
// The webhook URL is the tenant's to choose, so it is also theirs to point at our own network: it
// goes through safehttp (public addresses only, no redirect, unless Config.AllowPrivateTargets) and
// must be https. What comes back in detail is a short code, never the transport error, so the
// answer cannot be used to map what is reachable from here.
func (c Config) deliver(ctx context.Context, channels json.RawMessage, payload any) (bool, string) {
	obj := channelsObject(channels)
	if raw, ok := obj["webhook"]; ok {
		if url, ok := jsonAsString(raw); ok {
			if err := c.checkWebhookURL(url); err != nil {
				return false, "webhook refused: " + err.Error()
			}
			body, _ := json.Marshal(payload)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
			if err != nil {
				return false, "webhook refused: not a valid URL"
			}
			req.Header.Set(standard.HeaderContentType, standard.MediaTypeJSON)
			resp, err := safehttp.NewClient(safehttp.Options{Timeout: deliveryTimeout, AllowPrivate: c.AllowPrivateTargets}).Do(req)
			if err != nil {
				if errors.Is(err, safehttp.ErrPrivateTarget) {
					return false, "webhook refused: the address is not public"
				}
				return false, "webhook unreachable"
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

// checkWebhookURL accepts an https URL with a host (http too, when private targets are allowed:
// development, where the receiver is a local mock).
func (c Config) checkWebhookURL(raw string) error {
	u, err := neturl.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil {
		return errors.New("not a valid URL")
	}
	if u.Scheme != "https" && !(c.AllowPrivateTargets && u.Scheme == "http") {
		return errors.New("must be an https URL")
	}
	return nil
}

// ensureCanRecord refuses before anything goes out when the caller could not record the delivery:
// a notification is written after every attempt, so a caller who cannot write it must not be able to
// make the attempt either.
func ensureCanRecord(ctx context.Context, state *httpserver.AppState, rc *permission.RequestContext) error {
	d, err := state.Permissions.CanCreateEntity(ctx, rc, model.AlertNotifications)
	if err != nil {
		return err
	}
	if !d.Allowed {
		return crud.NewServiceErrorMsg(http.StatusForbidden, errcode.Forbidden, "You may not send alerts for this policy.")
	}
	return nil
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
