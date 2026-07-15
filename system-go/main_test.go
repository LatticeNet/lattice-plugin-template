package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// failHost fails the test if the plugin reaches for the host on a path that should never
// need it (describe, health, plan). Only execute talks to the host.
type failHost struct{ t *testing.T }

func (h failHost) call(method string, _ any) (json.RawMessage, error) {
	h.t.Helper()
	h.t.Fatalf("this path must not call the host, but it called %q", method)
	return nil, nil
}

func offlineRuntime(t *testing.T) *runtime { return &runtime{host: failHost{t}} }

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
	resp := offlineRuntime(t).handle(request{Action: "describe"})
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
	if body.ID != manifest.ID || body.Name != manifest.Name || body.Version != manifest.Version {
		t.Fatalf("describe %+v does not match manifest %+v", body, manifest)
	}
}

func TestHealthReportsReady(t *testing.T) {
	resp := offlineRuntime(t).handle(request{Action: "health"})
	if !resp.OK || !strings.Contains(resp.Message, "healthy") {
		t.Fatalf("health = %+v", resp)
	}
}

// The plan-effect method returns a PluginOperationPlan the server can bound and store as
// a pending approval. It names the operator's targets and applies nothing itself.
func TestPlanReturnsAnOperationPlan(t *testing.T) {
	resp := offlineRuntime(t).handle(request{
		Action: "call", Service: referenceService, Method: "plan",
		Payload: json.RawMessage(`{"targets":["node-a","node-b"]}`),
	})
	if !resp.OK {
		t.Fatalf("plan ok = false, error = %q", resp.Error)
	}
	var plan operationPlan
	if err := json.Unmarshal(resp.Result, &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Summary == "" || len(plan.Targets) != 2 || plan.Targets[0] != "node-a" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if plan.Rollback == "" {
		t.Fatal("a reviewable plan must state its rollback")
	}
}

// An empty-target plan is a validation error, not an unsupported method: the method is
// served, the input is incomplete. This is what keeps the conformance probe honest.
func TestPlanRejectsEmptyTargets(t *testing.T) {
	resp := offlineRuntime(t).handle(request{
		Action: "call", Service: referenceService, Method: "plan",
		Payload: json.RawMessage(`{}`),
	})
	if resp.OK {
		t.Fatal("a plan with no targets must not succeed")
	}
	if strings.Contains(resp.Error, "unsupported") {
		t.Fatalf("empty targets must be a validation error, not unsupported: %q", resp.Error)
	}
}

// recordHost captures task.enqueue calls so the execute flow can be verified without a
// real runner.
type recordHost struct {
	calls  []taskEnqueueParams
	failOn string // node id to reject, mimicking a host refusal
}

func (h *recordHost) call(method string, params any) (json.RawMessage, error) {
	if method != "task.enqueue" {
		return nil, nil
	}
	raw, _ := json.Marshal(params)
	var p taskEnqueueParams
	_ = json.Unmarshal(raw, &p)
	if p.NodeID == h.failOn {
		return nil, &hostRefusal{p.NodeID}
	}
	h.calls = append(h.calls, p)
	return json.RawMessage(`{"task_id":"task-1"}`), nil
}

type hostRefusal struct{ node string }

func (e *hostRefusal) Error() string {
	return "node " + e.node + " is not among the approved targets"
}

// execute enqueues one bounded task per approved target and never applies anything
// itself.
func TestExecuteEnqueuesOneTaskPerTarget(t *testing.T) {
	host := &recordHost{}
	rt := &runtime{host: host}
	resp := rt.handle(request{
		Action:  "execute",
		Payload: json.RawMessage(`{"approval_id":"appr-1","targets":["node-a","node-b"]}`),
	})
	if !resp.OK {
		t.Fatalf("execute ok = false, error = %q", resp.Error)
	}
	if len(host.calls) != 2 {
		t.Fatalf("want one task per target, got %d", len(host.calls))
	}
	for i, node := range []string{"node-a", "node-b"} {
		if host.calls[i].NodeID != node {
			t.Fatalf("task %d aimed at %q, want %q", i, host.calls[i].NodeID, node)
		}
		if host.calls[i].Interpreter != "sh" || host.calls[i].Script == "" {
			t.Fatalf("task %d malformed: %+v", i, host.calls[i])
		}
	}
}

// A host refusal — an unapproved node, an exhausted grant, the kill switch — surfaces
// verbatim; the plugin does not get to override it.
func TestExecuteSurfacesHostRefusal(t *testing.T) {
	host := &recordHost{failOn: "node-b"}
	rt := &runtime{host: host}
	resp := rt.handle(request{
		Action:  "execute",
		Payload: json.RawMessage(`{"approval_id":"appr-1","targets":["node-a","node-b"]}`),
	})
	if resp.OK {
		t.Fatal("execute must fail when the host refuses a task")
	}
	if !strings.Contains(resp.Error, "node-b") {
		t.Fatalf("the refusal must surface: %q", resp.Error)
	}
}

func TestExecuteInjectionSafeShellQuoting(t *testing.T) {
	host := &recordHost{}
	rt := &runtime{host: host}
	resp := rt.handle(request{
		Action:  "execute",
		Payload: json.RawMessage(`{"approval_id":"a'; rm -rf /; echo '","targets":["node-a"]}`),
	})
	if !resp.OK {
		t.Fatalf("execute ok = false: %q", resp.Error)
	}
	// The injected closing quote is escaped, so the malicious text stays a string literal
	// and no second command exists.
	if !strings.Contains(host.calls[0].Script, `'\''`) {
		t.Fatalf("approval id was not shell-quoted: %s", host.calls[0].Script)
	}
}

func TestRenderPlanIsDeterministicAndNonMutating(t *testing.T) {
	plan := renderPlan(json.RawMessage(`{"public_tcp":[80,443],"node_id":"node-a"}`))
	nodeAt := strings.Index(plan, "# node_id = node-a")
	tcpAt := strings.Index(plan, "# public_tcp =")
	if nodeAt < 0 || tcpAt < 0 || nodeAt > tcpAt {
		t.Fatalf("plan keys missing or unsorted:\n%s", plan)
	}
	if !strings.Contains(plan, "No host changes are made by this template.") {
		t.Fatalf("plan must state dry-run behavior:\n%s", plan)
	}
}

func TestUnsupportedActionFailsClosed(t *testing.T) {
	resp := offlineRuntime(t).handle(request{Action: "apply"})
	if resp.OK || !strings.Contains(resp.Error, `unsupported action "apply"`) {
		t.Fatalf("unexpected: %+v", resp)
	}
}

func TestCallActionFailsClosedForUnknownServiceOrMethod(t *testing.T) {
	rt := offlineRuntime(t)
	serviceResp := rt.handle(request{Action: "call", Service: "example.lattice-plugin/other", Method: "plan"})
	if serviceResp.OK || !strings.Contains(serviceResp.Error, "unsupported service") {
		t.Fatalf("unknown service: %+v", serviceResp)
	}
	methodResp := rt.handle(request{Action: "call", Service: referenceService, Method: "apply"})
	if methodResp.OK || !strings.Contains(methodResp.Error, "unsupported method") {
		t.Fatalf("unknown method: %+v", methodResp)
	}
}
