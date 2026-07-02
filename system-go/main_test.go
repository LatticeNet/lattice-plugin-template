package main

import (
	"strings"
	"testing"
)

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
