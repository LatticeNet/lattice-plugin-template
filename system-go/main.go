package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

const (
	pluginID      = "example.lattice-plugin"
	pluginName    = "Lattice Bundle Reference Plugin"
	pluginVersion = "0.2.1-alpha.1"
)

var interfaces = []string{"example.describe", "example.plan"}
var requiredScopes = []string{"network:plan"}

type request struct {
	Action  string         `json:"action"`
	Service string         `json:"service,omitempty"`
	Method  string         `json:"method,omitempty"`
	Payload map[string]any `json:"payload"`
}

type response struct {
	OK      bool            `json:"ok"`
	Plan    string          `json:"plan,omitempty"`
	Message string          `json:"message,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   string          `json:"error,omitempty"`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		var req request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			write(response{OK: false, Error: "invalid request: " + err.Error()})
			continue
		}
		write(handle(req))
	}
}

func handle(req request) response {
	switch req.Action {
	case "describe":
		body, _ := json.Marshal(describeBody())
		return response{OK: true, Result: body, Message: "reference plugin interface surface"}
	case "health":
		return response{OK: true, Message: "example plugin healthy"}
	case "plan":
		return response{OK: true, Plan: renderPlan(req.Payload), Message: "dry-run plan generated"}
	case "call":
		return handleCall(req)
	default:
		return response{OK: false, Error: fmt.Sprintf("unsupported action %q", req.Action)}
	}
}

func handleCall(req request) response {
	if req.Service != "example.lattice-plugin/reference" {
		return response{OK: false, Error: fmt.Sprintf("unsupported service %q", req.Service)}
	}

	switch req.Method {
	case "describe":
		body, _ := json.Marshal(describeBody())
		return response{OK: true, Result: body, Message: "describe result generated"}
	case "plan":
		body, _ := json.Marshal(map[string]any{
			"plan": renderPlan(req.Payload),
		})
		return response{OK: true, Result: body, Message: "plan result generated"}
	default:
		return response{OK: false, Error: fmt.Sprintf("unsupported method %q", req.Method)}
	}
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

func write(resp response) {
	_ = json.NewEncoder(os.Stdout).Encode(resp)
}
