package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	latticeplugin "github.com/LatticeNet/lattice-sdk/plugin"
)

const (
	pluginID      = "example.lattice-plugin"
	pluginName    = "Lattice Bundle Reference Plugin"
	pluginVersion = "0.2.1-alpha.6"
)

var capabilities = []string{"network:plan", "http:operator-target"}
var interfaces = []string{
	"example.lattice-plugin/reference.describe",
	"example.lattice-plugin/reference.plan",
	"example.lattice-plugin/reference.probe_operator_target",
}
var requiredScopes = []string{"network:plan"}

type request = latticeplugin.Request
type callPayload = latticeplugin.CallPayload
type response = latticeplugin.Response

type operatorTargetRequest struct {
	BaseURL string `json:"base_url"`
}

func main() {
	rt := &runtime{}
	_ = latticeplugin.Serve(context.Background(), latticeplugin.HandlerFunc(
		func(ctx context.Context, req latticeplugin.Request, host *latticeplugin.HostClient) latticeplugin.Response {
			rt.host = sdkHostCaller{ctx: ctx, client: host}
			return rt.handle(req)
		},
	))
}

type runtime struct {
	host hostCaller
}

type hostCaller interface {
	call(method string, params any) (json.RawMessage, error)
}

type sdkHostCaller struct {
	ctx    context.Context
	client *latticeplugin.HostClient
}

func (host sdkHostCaller) call(method string, params any) (json.RawMessage, error) {
	return host.client.Call(host.ctx, method, params)
}

func handle(req request) response {
	return (&runtime{}).handle(req)
}

func (rt *runtime) handle(req request) response {
	switch req.Action {
	case latticeplugin.ActionDescribe:
		body, _ := json.Marshal(describeBody())
		return latticeplugin.RawResultResponse(body, "reference plugin interface surface")
	case latticeplugin.ActionHealth:
		return latticeplugin.MessageResponse("example plugin healthy")
	case latticeplugin.ActionPlan:
		plan, err := renderPlan(req.Payload)
		if err != nil {
			return latticeplugin.ErrorResponse(err)
		}
		return latticeplugin.PlanResponse(plan, "dry-run plan generated")
	case latticeplugin.ActionCall:
		return rt.handleCall(req)
	default:
		return latticeplugin.ErrorResponse(fmt.Errorf("unsupported action %q", req.Action))
	}
}

func (rt *runtime) handleCall(req request) response {
	call, err := req.CallPayload()
	if err != nil {
		return latticeplugin.ErrorResponse(fmt.Errorf("invalid call payload: %w", err))
	}
	if call.Service != pluginID+"/reference" {
		return latticeplugin.ErrorResponse(fmt.Errorf("unsupported service %q", call.Service))
	}

	switch call.Method {
	case "describe":
		body, _ := json.Marshal(describeBody())
		return latticeplugin.RawResultResponse(body, "describe result generated")
	case "plan":
		plan, err := renderPlan(call.Payload)
		if err != nil {
			return latticeplugin.ErrorResponse(err)
		}
		body, _ := json.Marshal(map[string]any{"plan": plan})
		return latticeplugin.RawResultResponse(body, "plan result generated")
	case "probe_operator_target":
		return rt.probeOperatorTarget(call.Payload)
	default:
		return latticeplugin.ErrorResponse(fmt.Errorf("unsupported method %q", call.Method))
	}
}

func describeBody() map[string]any {
	return map[string]any{
		"id":              pluginID,
		"name":            pluginName,
		"version":         pluginVersion,
		"capabilities":    capabilities,
		"interfaces":      interfaces,
		"required_scopes": requiredScopes,
		"manages": []string{
			"example deterministic dry-run plans",
			"self-contained bundle packaging and sandbox bridge patterns",
			"host-routed operator target probes with no direct sockets",
		},
		"engine": "bundle v2 stdio-json-v1 system runtime with SDK host calls",
	}
}

func (rt *runtime) probeOperatorTarget(payload json.RawMessage) response {
	var req operatorTargetRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return latticeplugin.ErrorResponse(fmt.Errorf("invalid operator target payload: %w", err))
	}
	target, err := normalizeOperatorTargetURL(req.BaseURL)
	if err != nil {
		return latticeplugin.ErrorResponse(err)
	}
	raw, err := rt.callHost(latticeplugin.HostMethodHTTPOperatorDo, map[string]any{
		"method": "GET",
		"url":    target,
	})
	if err != nil {
		return latticeplugin.ErrorResponse(fmt.Errorf("operator target probe failed"))
	}
	var hostResp struct {
		StatusCode int `json:"status_code"`
	}
	if err := json.Unmarshal(raw, &hostResp); err != nil {
		return latticeplugin.ErrorResponse(fmt.Errorf("decode operator target response: %w", err))
	}
	body, _ := json.Marshal(map[string]any{
		"reachable":   hostResp.StatusCode >= 200 && hostResp.StatusCode < 500,
		"status_code": hostResp.StatusCode,
	})
	return latticeplugin.RawResultResponse(body, "operator target probe complete")
}

func (rt *runtime) callHost(method string, params any) (json.RawMessage, error) {
	if rt.host == nil {
		return nil, fmt.Errorf("host response fd unavailable")
	}
	return rt.host.call(method, params)
}

func renderPlan(payload json.RawMessage) (string, error) {
	values, err := decodeObjectPayload(payload)
	if err != nil {
		return "", err
	}
	parts := []string{"# Example Lattice system plugin plan"}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := values[key]
		parts = append(parts, fmt.Sprintf("# %s = %v", key, value))
	}
	parts = append(parts, "# No host changes are made by this template.")
	return strings.Join(parts, "\n"), nil
}

func decodeObjectPayload(payload json.RawMessage) (map[string]any, error) {
	if len(strings.TrimSpace(string(payload))) == 0 {
		return map[string]any{}, nil
	}
	var values map[string]any
	if err := json.Unmarshal(payload, &values); err != nil {
		return nil, fmt.Errorf("payload must be a JSON object: %w", err)
	}
	if values == nil {
		return map[string]any{}, nil
	}
	return values, nil
}

func normalizeOperatorTargetURL(value string) (string, error) {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return "", fmt.Errorf("base_url is required")
	}
	if len(raw) > 2048 || hasControl(raw) {
		return "", fmt.Errorf("base_url must be printable and at most 2048 characters")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("base_url must be an absolute http(s) URL")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return "", fmt.Errorf("base_url must use http or https")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("base_url must not include credentials")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("base_url must not include query or fragment")
	}
	return parsed.String(), nil
}

func hasControl(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
