package main

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

type manifestContract struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities"`
}

func TestDescribeMatchesManifestContract(t *testing.T) {
	raw, err := os.ReadFile("../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest manifestContract
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}

	resp := handle(request{Action: "describe"})
	if !resp.OK {
		t.Fatalf("describe ok = false, error = %q", resp.Error)
	}
	var body struct {
		ID           string   `json:"id"`
		Name         string   `json:"name"`
		Version      string   `json:"version"`
		Capabilities []string `json:"capabilities"`
	}
	if err := json.Unmarshal(resp.Result, &body); err != nil {
		t.Fatal(err)
	}
	if body.ID != manifest.ID {
		t.Fatalf("describe id = %q, manifest id = %q", body.ID, manifest.ID)
	}
	if body.Name != manifest.Name {
		t.Fatalf("describe name = %q, manifest name = %q", body.Name, manifest.Name)
	}
	if body.Version != manifest.Version {
		t.Fatalf("describe version = %q, manifest version = %q", body.Version, manifest.Version)
	}
	if !reflect.DeepEqual(body.Capabilities, manifest.Capabilities) {
		t.Fatalf("describe capabilities = %v, manifest capabilities = %v", body.Capabilities, manifest.Capabilities)
	}
}

func TestHealthReportsReady(t *testing.T) {
	resp := handle(request{Action: "health"})

	if !resp.OK {
		t.Fatalf("health ok = false, error = %q", resp.Error)
	}
	if !strings.Contains(resp.Message, "healthy") {
		t.Fatalf("health message = %q, want healthy", resp.Message)
	}
}

func TestRenderPlanIsDeterministicAndNonMutating(t *testing.T) {
	plan := renderPlan(map[string]any{
		"public_tcp": []any{80, 443},
		"node_id":    "node-a",
	})

	nodeAt := strings.Index(plan, "# node_id = node-a")
	tcpAt := strings.Index(plan, "# public_tcp = [80 443]")
	if nodeAt < 0 || tcpAt < 0 {
		t.Fatalf("plan missing expected keys:\n%s", plan)
	}
	if nodeAt > tcpAt {
		t.Fatalf("plan keys are not sorted:\n%s", plan)
	}
	if !strings.Contains(plan, "No host changes are made by this template.") {
		t.Fatalf("plan must state dry-run behavior:\n%s", plan)
	}
}

func TestUnsupportedActionFailsClosed(t *testing.T) {
	resp := handle(request{Action: "apply"})

	if resp.OK {
		t.Fatal("unsupported action returned ok=true")
	}
	if !strings.Contains(resp.Error, `unsupported action "apply"`) {
		t.Fatalf("unexpected error: %q", resp.Error)
	}
}
