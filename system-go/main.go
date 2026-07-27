package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
)

const (
	pluginID      = "example.lattice-plugin"
	pluginName    = "Lattice Bundle Reference Plugin"
	pluginVersion = "0.2.1-alpha.3"
)

var capabilities = []string{"network:plan", "http:operator-target"}
var interfaces = []string{
	"example.lattice-plugin/reference.describe",
	"example.lattice-plugin/reference.plan",
	"example.lattice-plugin/reference.probe_operator_target",
}
var requiredScopes = []string{"network:plan"}

type request struct {
	Action  string          `json:"action"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type callPayload struct {
	Service string          `json:"service"`
	Method  string          `json:"method"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type operatorTargetRequest struct {
	BaseURL string `json:"base_url"`
}

type response struct {
	OK      bool            `json:"ok"`
	Plan    string          `json:"plan,omitempty"`
	Message string          `json:"message,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   string          `json:"error,omitempty"`
}

type hostCallEnvelope struct {
	HostCall hostCall `json:"host_call"`
}

type hostCall struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

type hostResponseEnvelope struct {
	HostResponse hostResponse `json:"host_response"`
}

type hostResponse struct {
	ID     string          `json:"id"`
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result"`
	Error  string          `json:"error"`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	respScanner, closeResponses := hostResponseScanner()
	defer closeResponses()
	rt := &runtime{host: &stdioHostCaller{responses: respScanner, output: os.Stdout}}
	for scanner.Scan() {
		var req request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			write(response{OK: false, Error: "invalid request: " + err.Error()})
			continue
		}
		write(rt.handle(req))
	}
}

type runtime struct {
	host hostCaller
}

type hostCaller interface {
	call(method string, params any) (json.RawMessage, error)
}

type stdioHostCaller struct {
	responses *bufio.Scanner
	nextID    int
	output    io.Writer
}

func handle(req request) response {
	return (&runtime{}).handle(req)
}

func (rt *runtime) handle(req request) response {
	switch req.Action {
	case "describe":
		body, _ := json.Marshal(describeBody())
		return response{OK: true, Result: body, Message: "reference plugin interface surface"}
	case "health":
		return response{OK: true, Message: "example plugin healthy"}
	case "plan":
		plan, err := renderPlan(req.Payload)
		if err != nil {
			return response{OK: false, Error: err.Error()}
		}
		return response{OK: true, Plan: plan, Message: "dry-run plan generated"}
	case "call":
		return rt.handleCall(req.Payload)
	default:
		return response{OK: false, Error: fmt.Sprintf("unsupported action %q", req.Action)}
	}
}

func (rt *runtime) handleCall(payload json.RawMessage) response {
	var call callPayload
	if err := json.Unmarshal(payload, &call); err != nil {
		return response{OK: false, Error: "invalid call payload: " + err.Error()}
	}
	if call.Service != pluginID+"/reference" {
		return response{OK: false, Error: fmt.Sprintf("unsupported service %q", call.Service)}
	}

	switch call.Method {
	case "describe":
		body, _ := json.Marshal(describeBody())
		return response{OK: true, Result: body, Message: "describe result generated"}
	case "plan":
		plan, err := renderPlan(call.Payload)
		if err != nil {
			return response{OK: false, Error: err.Error()}
		}
		body, _ := json.Marshal(map[string]any{
			"plan": plan,
		})
		return response{OK: true, Result: body, Message: "plan result generated"}
	case "probe_operator_target":
		return rt.probeOperatorTarget(call.Payload)
	default:
		return response{OK: false, Error: fmt.Sprintf("unsupported method %q", call.Method)}
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
		"engine": "bundle v2 stdio-json-v1 system runtime with fd-3 host calls",
	}
}

func (rt *runtime) probeOperatorTarget(payload json.RawMessage) response {
	var req operatorTargetRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return response{OK: false, Error: "invalid operator target payload: " + err.Error()}
	}
	target, err := normalizeOperatorTargetURL(req.BaseURL)
	if err != nil {
		return response{OK: false, Error: err.Error()}
	}
	raw, err := rt.callHost("http.operator.do", map[string]any{
		"method": "GET",
		"url":    target,
	})
	if err != nil {
		return response{OK: false, Error: "operator target probe failed"}
	}
	var hostResp struct {
		StatusCode int `json:"status_code"`
	}
	if err := json.Unmarshal(raw, &hostResp); err != nil {
		return response{OK: false, Error: "decode operator target response: " + err.Error()}
	}
	body, _ := json.Marshal(map[string]any{
		"reachable":   hostResp.StatusCode >= 200 && hostResp.StatusCode < 500,
		"status_code": hostResp.StatusCode,
	})
	return response{OK: true, Result: body, Message: "operator target probe complete"}
}

func (rt *runtime) callHost(method string, params any) (json.RawMessage, error) {
	if rt.host == nil {
		return nil, fmt.Errorf("host response fd unavailable")
	}
	return rt.host.call(method, params)
}

func (host *stdioHostCaller) call(method string, params any) (json.RawMessage, error) {
	if host == nil || host.responses == nil || host.output == nil {
		return nil, fmt.Errorf("host response fd unavailable")
	}
	host.nextID++
	id := fmt.Sprintf("h%d", host.nextID)
	if err := json.NewEncoder(host.output).Encode(hostCallEnvelope{
		HostCall: hostCall{ID: id, Method: method, Params: params},
	}); err != nil {
		return nil, fmt.Errorf("write host_call: %w", err)
	}
	if !host.responses.Scan() {
		if err := host.responses.Err(); err != nil {
			return nil, fmt.Errorf("read host_response: %w", err)
		}
		return nil, fmt.Errorf("read host_response: eof")
	}
	var env hostResponseEnvelope
	if err := json.Unmarshal(host.responses.Bytes(), &env); err != nil {
		return nil, fmt.Errorf("decode host_response: %w", err)
	}
	if env.HostResponse.ID != id {
		return nil, fmt.Errorf("host_response id mismatch: got %q want %q", env.HostResponse.ID, id)
	}
	if !env.HostResponse.OK {
		if env.HostResponse.Error == "" {
			env.HostResponse.Error = "host call failed"
		}
		return nil, fmt.Errorf("%s: %s", method, env.HostResponse.Error)
	}
	return env.HostResponse.Result, nil
}

func hostResponseScanner() (*bufio.Scanner, func()) {
	fd := 3
	if raw := strings.TrimSpace(os.Getenv("LATTICE_HOST_RESPONSE_FD")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 3 {
			return nil, func() {}
		}
		fd = parsed
	}
	file := os.NewFile(uintptr(fd), "lattice-host-response")
	if file == nil {
		return nil, func() {}
	}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	return scanner, func() { _ = file.Close() }
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

func write(resp response) {
	_ = json.NewEncoder(os.Stdout).Encode(resp)
}
