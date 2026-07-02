package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

type request struct {
	Action  string         `json:"action"`
	Payload map[string]any `json:"payload"`
}

type response struct {
	OK      bool   `json:"ok"`
	Plan    string `json:"plan,omitempty"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
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
