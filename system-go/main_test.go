package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type manifestContract struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
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
		ID      string `json:"id"`
		Name    string `json:"name"`
		Version string `json:"version"`
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

func TestCallActionSupportsReferenceDescribeAndPlan(t *testing.T) {
	describeResp := handle(request{
		Action:  "call",
		Service: "example.lattice-plugin/reference",
		Method:  "describe",
	})
	if !describeResp.OK {
		t.Fatalf("call describe ok = false, error = %q", describeResp.Error)
	}
	var describeBody struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(describeResp.Result, &describeBody); err != nil {
		t.Fatal(err)
	}
	if describeBody.ID != pluginID || describeBody.Name != pluginName || describeBody.Version != pluginVersion {
		t.Fatalf("unexpected describe body: %+v", describeBody)
	}

	planResp := handle(request{
		Action:  "call",
		Service: "example.lattice-plugin/reference",
		Method:  "plan",
		Payload: map[string]any{
			"node_id":    "node-a",
			"public_tcp": []any{80, 443},
		},
	})
	if !planResp.OK {
		t.Fatalf("call plan ok = false, error = %q", planResp.Error)
	}
	var planBody struct {
		Plan string `json:"plan"`
	}
	if err := json.Unmarshal(planResp.Result, &planBody); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(planBody.Plan, "# node_id = node-a") {
		t.Fatalf("plan result missing node id:\n%s", planBody.Plan)
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

func TestCallActionFailsClosedForUnknownServiceOrMethod(t *testing.T) {
	serviceResp := handle(request{
		Action:  "call",
		Service: "example.lattice-plugin/other",
		Method:  "plan",
	})
	if serviceResp.OK {
		t.Fatal("unknown service returned ok=true")
	}
	if !strings.Contains(serviceResp.Error, "unsupported service") {
		t.Fatalf("unexpected service error: %q", serviceResp.Error)
	}

	methodResp := handle(request{
		Action:  "call",
		Service: "example.lattice-plugin/reference",
		Method:  "apply",
	})
	if methodResp.OK {
		t.Fatal("unknown method returned ok=true")
	}
	if !strings.Contains(methodResp.Error, "unsupported method") {
		t.Fatalf("unexpected method error: %q", methodResp.Error)
	}
}
