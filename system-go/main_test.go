package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	latticeplugin "github.com/LatticeNet/lattice-sdk/plugin"
)

type manifestContract struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities"`
	Interfaces   []struct {
		Service string `json:"service"`
		Backing string `json:"backing"`
		Methods []struct {
			Name                 string   `json:"name"`
			OperatorTargetFields []string `json:"operator_target_fields"`
		} `json:"methods"`
	} `json:"interfaces"`
}

func TestDescribeMatchesManifestContract(t *testing.T) {
	manifest := readManifest(t)

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
	if !sameStrings(body.Capabilities, manifest.Capabilities) {
		t.Fatalf("describe capabilities = %+v, manifest capabilities = %+v", body.Capabilities, manifest.Capabilities)
	}
}

func TestManifestDeclaresRuntimeBackingAndOperatorTargetBinding(t *testing.T) {
	manifest := readManifest(t)

	if len(manifest.Interfaces) != 1 {
		t.Fatalf("manifest interfaces = %d, want 1", len(manifest.Interfaces))
	}
	if manifest.Interfaces[0].Backing != "runtime" {
		t.Fatalf("manifest backing = %q, want runtime", manifest.Interfaces[0].Backing)
	}
	if !contains(manifest.Capabilities, "http:operator-target") {
		t.Fatal("manifest must declare http:operator-target for the operator probe method")
	}

	found := false
	for _, method := range manifest.Interfaces[0].Methods {
		if method.Name != "probe_operator_target" {
			continue
		}
		found = true
		if !sameStrings(method.OperatorTargetFields, []string{"base_url"}) {
			t.Fatalf("operator target fields = %+v, want [base_url]", method.OperatorTargetFields)
		}
	}
	if !found {
		t.Fatal("manifest missing probe_operator_target method")
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
	plan, err := renderPlan(mustRaw(t, map[string]any{
		"public_tcp": []any{80, 443},
		"node_id":    "node-a",
	}))
	if err != nil {
		t.Fatal(err)
	}

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

func TestCallActionSupportsManifestDeclaredRuntimeMethods(t *testing.T) {
	manifest := readManifest(t)
	host := &fakeHostCaller{responses: []json.RawMessage{json.RawMessage(`{"status_code":204}`)}}
	rt := &runtime{host: host}

	for _, iface := range manifest.Interfaces {
		if iface.Backing != "runtime" {
			continue
		}
		for _, method := range iface.Methods {
			payload := json.RawMessage(`{}`)
			if method.Name == "plan" {
				payload = mustRaw(t, map[string]any{
					"node_id":    "node-a",
					"public_tcp": []any{80, 443},
				})
			}
			if method.Name == "probe_operator_target" {
				payload = mustRaw(t, map[string]any{
					"base_url": "http://127.0.0.1:3000/health",
				})
			}
			resp := rt.handle(request{
				Action: "call",
				Payload: mustRaw(t, callPayload{
					Service: iface.Service,
					Method:  method.Name,
					Payload: payload,
				}),
			})
			if !resp.OK {
				t.Fatalf("%s/%s ok = false, error = %q", iface.Service, method.Name, resp.Error)
			}
		}
	}

	if len(host.calls) != 1 || host.calls[0].method != latticeplugin.HostMethodHTTPOperatorDo {
		t.Fatalf("operator probe should use exactly one host call, got %+v", host.calls)
	}
	if got := host.calls[0].params["url"]; got != "http://127.0.0.1:3000/health" {
		t.Fatalf("operator probe url = %v", got)
	}
}

func TestOperatorTargetProbeFailsClosedBeforeHostCall(t *testing.T) {
	host := &fakeHostCaller{}
	rt := &runtime{host: host}

	resp := rt.handle(request{
		Action: "call",
		Payload: mustRaw(t, callPayload{
			Service: pluginID + "/reference",
			Method:  "probe_operator_target",
			Payload: mustRaw(t, map[string]any{"base_url": "file:///etc/passwd"}),
		}),
	})
	if resp.OK {
		t.Fatal("invalid operator target returned ok=true")
	}
	if !strings.Contains(resp.Error, "absolute http(s) URL") {
		t.Fatalf("unexpected error: %q", resp.Error)
	}
	if len(host.calls) != 0 {
		t.Fatalf("invalid operator target reached host: %+v", host.calls)
	}

	secretURL := "http://127.0.0.1:3000/very-secret"
	host = &fakeHostCaller{errs: []error{errors.New("dial " + secretURL + ": refused")}}
	resp = (&runtime{host: host}).handle(request{
		Action: "call",
		Payload: mustRaw(t, callPayload{
			Service: pluginID + "/reference",
			Method:  "probe_operator_target",
			Payload: mustRaw(t, map[string]any{"base_url": secretURL}),
		}),
	})
	if resp.OK {
		t.Fatal("host failure returned ok=true")
	}
	if strings.Contains(resp.Error, "very-secret") || resp.Error != "operator target probe failed" {
		t.Fatalf("operator target error leaked host detail: %q", resp.Error)
	}
}

func TestSDKHostClientRoundTrip(t *testing.T) {
	responses := strings.NewReader(`{"host_response":{"id":"h1","ok":true,"result":{"status_code":204}}}` + "\n")
	var output bytes.Buffer
	host := latticeplugin.NewHostClient(latticeplugin.HostClientOptions{
		Output:    &output,
		Responses: responses,
	})

	raw, err := host.Call(context.Background(), latticeplugin.HostMethodHTTPOperatorDo, map[string]any{
		"method": "GET",
		"url":    "http://127.0.0.1:3000/health",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"status_code":204`)) {
		t.Fatalf("host result = %s, want status_code 204", raw)
	}
	var out struct {
		HostCall struct {
			ID     string         `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		} `json:"host_call"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &out); err != nil {
		t.Fatalf("decode host call %q: %v", output.String(), err)
	}
	if out.HostCall.ID != "h1" || out.HostCall.Method != latticeplugin.HostMethodHTTPOperatorDo {
		t.Fatalf("host call envelope = %+v", out.HostCall)
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
		Action: "call",
		Payload: mustRaw(t, callPayload{
			Service: "example.lattice-plugin/other",
			Method:  "plan",
		}),
	})
	if serviceResp.OK {
		t.Fatal("unknown service returned ok=true")
	}
	if !strings.Contains(serviceResp.Error, "unsupported service") {
		t.Fatalf("unexpected service error: %q", serviceResp.Error)
	}

	methodResp := handle(request{
		Action: "call",
		Payload: mustRaw(t, callPayload{
			Service: "example.lattice-plugin/reference",
			Method:  "apply",
		}),
	})
	if methodResp.OK {
		t.Fatal("unknown method returned ok=true")
	}
	if !strings.Contains(methodResp.Error, "unsupported method") {
		t.Fatalf("unexpected method error: %q", methodResp.Error)
	}
}

type recordedHostCall struct {
	method string
	params map[string]any
}

type fakeHostCaller struct {
	responses []json.RawMessage
	errs      []error
	calls     []recordedHostCall
}

func (f *fakeHostCaller) call(method string, params any) (json.RawMessage, error) {
	raw, _ := json.Marshal(params)
	decoded := map[string]any{}
	_ = json.Unmarshal(raw, &decoded)
	f.calls = append(f.calls, recordedHostCall{method: method, params: decoded})
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		return nil, err
	}
	if len(f.responses) == 0 {
		return nil, errors.New("missing fake host response")
	}
	out := f.responses[0]
	f.responses = f.responses[1:]
	return out, nil
}

func readManifest(t *testing.T) manifestContract {
	t.Helper()
	raw, err := os.ReadFile("../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest manifestContract
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func mustRaw(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
