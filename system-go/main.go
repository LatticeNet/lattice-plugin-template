// Command plugin is the reference Bundle V2 system runtime. It speaks the
// stdio-json-v1 protocol: one request object per stdin line, one response object per
// stdout line, and — for host calls — a {"host_call":{...}} line answered on the fd in
// LATTICE_HOST_RESPONSE_FD.
//
// It is deliberately runtime-backed and implements the full §9.3 host-risk flow end to
// end, so a new plugin has a correct shape to copy:
//
//   - a `plan`-effect interface method that returns a deterministic PluginOperationPlan
//     (the server turns it into a pending approval; nothing is applied);
//   - an `execute` action that the approval executor — and only it — invokes, which
//     enqueues bounded agent work through the task.enqueue host call.
//
// The plugin never applies anything itself and never sees the approval grant: it asks
// the host to enqueue a task, and the host refuses any task outside the approved plan.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

const (
	pluginID      = "example.lattice-plugin"
	pluginName    = "Lattice Bundle Reference Plugin"
	pluginVersion = "0.2.1-alpha.4"

	referenceService = "example.lattice-plugin/reference"
)

var interfaces = []string{"example.describe", "example.plan"}
var requiredScopes = []string{"network:plan"}

type request struct {
	Action  string          `json:"action"`
	Service string          `json:"service,omitempty"`
	Method  string          `json:"method,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type response struct {
	OK      bool            `json:"ok"`
	Plan    string          `json:"plan,omitempty"`
	Message string          `json:"message,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// hostCaller makes one brokered host call and returns its result. The runtime depends
// on this interface so tests can drive execute without a real host fd.
type hostCaller interface {
	call(method string, params any) (json.RawMessage, error)
}

type runtime struct {
	host hostCaller
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	responses, closeResponses := hostResponseScanner()
	defer closeResponses()
	rt := &runtime{host: &stdioHostCaller{responses: responses, output: os.Stdout}}
	for scanner.Scan() {
		var req request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			write(response{OK: false, Error: "invalid request: " + err.Error()})
			continue
		}
		write(rt.handle(req))
	}
}

func (rt *runtime) handle(req request) response {
	switch req.Action {
	case "describe":
		body, _ := json.Marshal(describeBody())
		return response{OK: true, Result: body, Message: "reference plugin interface surface"}
	case "health":
		return response{OK: true, Message: "example plugin healthy"}
	case "plan":
		return response{OK: true, Plan: renderPlan(req.Payload), Message: "dry-run plan generated"}
	case "call":
		return rt.handleCall(req)
	case "execute":
		// Reached ONLY from the server's approval executor, with an approved operation
		// grant bound to the invocation on the host side. There is no way for an
		// operator or a plugin author to reach this without an approval.
		return rt.handleExecute(req.Payload)
	default:
		return response{OK: false, Error: fmt.Sprintf("unsupported action %q", req.Action)}
	}
}

func (rt *runtime) handleCall(req request) response {
	if req.Service != referenceService {
		return response{OK: false, Error: fmt.Sprintf("unsupported service %q", req.Service)}
	}
	switch req.Method {
	case "describe":
		body, _ := json.Marshal(describeBody())
		return response{OK: true, Result: body, Message: "describe result generated"}
	case "plan":
		return rt.planOperation(req.Payload)
	default:
		return response{OK: false, Error: fmt.Sprintf("unsupported method %q", req.Method)}
	}
}

// planPayload is what an operator sends to the plan-effect method: the nodes they intend
// to act on. The plugin proposes; the server authorizes each target and stores the plan
// as a pending approval.
type planPayload struct {
	Targets []string `json:"targets"`
}

// operationPlan mirrors the server's plugin.PluginOperationPlan. The plan-effect method
// returns exactly this shape; the server unmarshals, bounds, and stores it.
type operationPlan struct {
	Summary  string          `json:"summary"`
	Targets  []string        `json:"targets"`
	Preview  string          `json:"preview,omitempty"`
	Steps    []string        `json:"steps,omitempty"`
	Rollback string          `json:"rollback,omitempty"`
	Data     json.RawMessage `json:"data,omitempty"`
}

func (rt *runtime) planOperation(payload json.RawMessage) response {
	var in planPayload
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &in); err != nil {
			return response{OK: false, Error: "invalid plan payload: " + err.Error()}
		}
	}
	if len(in.Targets) == 0 {
		// A validation error, not an "unsupported method": the method IS served, the
		// input is incomplete. The server would refuse an empty-target plan anyway.
		return response{OK: false, Error: "plan requires at least one target node"}
	}
	// The opaque data rides through approval back into execute unchanged. A real plugin
	// puts the compiled desired state here; the reference carries a marker.
	data, _ := json.Marshal(map[string]any{"reference": true})
	plan := operationPlan{
		Summary:  fmt.Sprintf("reference apply on %d node(s)", len(in.Targets)),
		Targets:  in.Targets,
		Preview:  "# reference plugin apply\n# writes nothing sensitive; enqueues one no-op task per node",
		Steps:    []string{"enqueue a bounded no-op task on each target"},
		Rollback: "none required; the reference task makes no host changes",
		Data:     data,
	}
	body, err := json.Marshal(plan)
	if err != nil {
		return response{OK: false, Error: "render plan: " + err.Error()}
	}
	return response{OK: true, Result: body, Message: "operation plan generated"}
}

// executeRequest is what the approval executor hands the plugin: the approved plan's
// opaque data and the approved targets. It is a convenience, not an authority — every
// task the plugin then enqueues is checked by the host against the invocation's grant.
type executeRequest struct {
	ApprovalID string          `json:"approval_id"`
	Targets    []string        `json:"targets"`
	Data       json.RawMessage `json:"data,omitempty"`
}

// taskEnqueueParams is the task.enqueue host-call shape. node_id must be one the
// operator approved, or the host refuses it.
type taskEnqueueParams struct {
	NodeID      string `json:"node_id"`
	Interpreter string `json:"interpreter"`
	Script      string `json:"script"`
	TimeoutSec  int    `json:"timeout_sec"`
}

func (rt *runtime) handleExecute(payload json.RawMessage) response {
	var req executeRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return response{OK: false, Error: "invalid execute payload: " + err.Error()}
	}
	if len(req.Targets) == 0 {
		return response{OK: false, Error: "execute received no targets"}
	}
	enqueued := 0
	for _, node := range req.Targets {
		script := fmt.Sprintf("echo 'lattice reference plugin applied approval %s on %s'",
			shellSingleQuote(req.ApprovalID), shellSingleQuote(node))
		if _, err := rt.host.call("task.enqueue", taskEnqueueParams{
			NodeID: node, Interpreter: "sh", Script: script, TimeoutSec: 60,
		}); err != nil {
			// A host refusal (unapproved node, exhausted grant, kill switch) surfaces here
			// verbatim; the plugin does not get to override it.
			return response{OK: false, Error: fmt.Sprintf("enqueue apply task on %s: %v", node, err)}
		}
		enqueued++
	}
	return response{OK: true, Message: fmt.Sprintf("enqueued %d reference apply task(s)", enqueued)}
}

// shellSingleQuote makes a value safe inside a single-quoted sh string, so an approval
// id or node id can never break out of the reference command.
func shellSingleQuote(v string) string {
	return strings.ReplaceAll(v, "'", `'\''`)
}

func describeBody() map[string]any {
	return map[string]any{
		"id":              pluginID,
		"name":            pluginName,
		"version":         pluginVersion,
		"interfaces":      interfaces,
		"required_scopes": requiredScopes,
		"manages": []string{
			"example deterministic operation plans",
			"the reference plan -> approve -> execute -> task.enqueue flow (spec §9.3)",
			"self-contained bundle packaging and sandbox bridge patterns",
		},
		"engine": "bundle v2 stdio-json-v1 system runtime",
	}
}

func renderPlan(payload json.RawMessage) string {
	values := map[string]any{}
	_ = json.Unmarshal(payload, &values)
	parts := []string{"# Example Lattice system plugin plan"}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("# %s = %v", key, values[key]))
	}
	parts = append(parts, "# No host changes are made by this template.")
	return strings.Join(parts, "\n")
}

func write(resp response) {
	_ = json.NewEncoder(os.Stdout).Encode(resp)
}

// --- host call client (stdio-json-v1) ------------------------------------------------

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

type stdioHostCaller struct {
	responses *bufio.Scanner
	nextID    int
	output    io.Writer
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
