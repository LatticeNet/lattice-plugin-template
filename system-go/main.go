package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
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
			write(response{OK: false, Error: err.Error()})
			continue
		}
		switch req.Action {
		case "plan":
			write(response{OK: true, Plan: renderPlan(req.Payload), Message: "dry-run plan generated"})
		default:
			write(response{OK: false, Error: "unsupported action"})
		}
	}
}

func renderPlan(payload map[string]any) string {
	parts := []string{"# Example Lattice system plugin plan"}
	for key, value := range payload {
		parts = append(parts, fmt.Sprintf("# %s = %v", key, value))
	}
	parts = append(parts, "# No host changes are made by this template.")
	return strings.Join(parts, "\n")
}

func write(resp response) {
	_ = json.NewEncoder(os.Stdout).Encode(resp)
}
