package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	latticeplugin "github.com/LatticeNet/lattice-sdk/plugin"
)

const (
	pluginID      = "example.lattice-plugin"
	pluginName    = "Lattice Bundle Reference Plugin"
	pluginVersion = "0.2.1-alpha.3"
)

var interfaces = []string{"example.describe", "example.plan"}
var requiredScopes = []string{"network:plan"}

type request = latticeplugin.Request
type response = latticeplugin.Response

func main() {
	_ = latticeplugin.Serve(context.Background(), latticeplugin.HandlerFunc(
		func(_ context.Context, req latticeplugin.Request, _ *latticeplugin.HostClient) latticeplugin.Response {
			return handle(req)
		},
	))
}

func handle(req request) response {
	switch req.Action {
	case latticeplugin.ActionDescribe:
		body, _ := json.Marshal(describeBody())
		return latticeplugin.RawResultResponse(body, "reference plugin interface surface")
	case latticeplugin.ActionHealth:
		return latticeplugin.MessageResponse("example plugin healthy")
	case latticeplugin.ActionPlan:
		payload, err := payloadMap(req.Payload)
		if err != nil {
			return latticeplugin.ErrorResponse(err)
		}
		return latticeplugin.PlanResponse(renderPlan(payload), "dry-run plan generated")
	case latticeplugin.ActionCall:
		return handleCall(req)
	default:
		return latticeplugin.ErrorResponse(fmt.Errorf("unsupported action %q", req.Action))
	}
}

func handleCall(req request) response {
	call, err := req.CallPayload()
	if err != nil {
		return latticeplugin.ErrorResponse(err)
	}
	if call.Service != "example.lattice-plugin/reference" {
		return latticeplugin.ErrorResponse(fmt.Errorf("unsupported service %q", call.Service))
	}

	switch call.Method {
	case "describe":
		body, _ := json.Marshal(describeBody())
		return latticeplugin.RawResultResponse(body, "describe result generated")
	case "plan":
		payload, err := payloadMap(call.Payload)
		if err != nil {
			return latticeplugin.ErrorResponse(err)
		}
		body, _ := json.Marshal(map[string]any{
			"plan": renderPlan(payload),
		})
		return latticeplugin.RawResultResponse(body, "plan result generated")
	default:
		return latticeplugin.ErrorResponse(fmt.Errorf("unsupported method %q", call.Method))
	}
}

func payloadMap(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}
	if payload == nil {
		payload = map[string]any{}
	}
	return payload, nil
}

func describeBody() map[string]any {
	return map[string]any{
		"id":              pluginID,
		"name":            pluginName,
		"version":         pluginVersion,
		"interfaces":      interfaces,
		"required_scopes": requiredScopes,
		"manages": []string{
			"example deterministic dry-run plans",
			"self-contained bundle packaging and sandbox bridge patterns",
		},
		"engine": "bundle v2 stdio-json-v1 system runtime",
	}
}

func renderPlan(payload map[string]any) string {
	parts := []string{"# Example Lattice system plugin plan"}
	keys := make([]string, 0, len(payload))
	for key := range payload {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := payload[key]
		parts = append(parts, fmt.Sprintf("# %s = %v", key, value))
	}
	parts = append(parts, "# No host changes are made by this template.")
	return strings.Join(parts, "\n")
}
