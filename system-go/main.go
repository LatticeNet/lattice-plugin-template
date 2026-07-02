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
	pluginName    = "Example Lattice Plugin"
	pluginVersion = "0.1.0"
)

var capabilities = []string{"network:plan"}

type request struct {
	Action  string         `json:"action"`
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
		body, _ := json.Marshal(map[string]any{
			"id":           pluginID,
			"name":         pluginName,
			"version":      pluginVersion,
			"capabilities": capabilities,
			"manages": []string{
				"example deterministic dry-run plans",
				"no host changes until the template is customized",
			},
			"engine": "template system-go stdio process",
		})
		return response{OK: true, Result: body, Message: "example plugin capability surface"}
	case "health":
		return response{OK: true, Message: "example plugin healthy"}
	case "plan":
		return response{OK: true, Plan: renderPlan(req.Payload), Message: "dry-run plan generated"}
	default:
		return response{OK: false, Error: fmt.Sprintf("unsupported action %q", req.Action)}
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
