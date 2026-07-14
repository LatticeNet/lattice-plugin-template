package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// A manifest declares which methods a plugin exposes and — since `backing` — who
// actually serves each one. This test holds the artifact to that promise.
//
// It exists because the promise used to go unchecked. Official plugins shipped signed
// manifests declaring interface methods their own artifacts could not answer, and
// lattice-server quietly answered them from an in-core handler instead. Nothing caught
// it: every suite covered what the artifact DOES, never what the manifest CLAIMS. A
// contract nobody verifies is a contract that drifts, and this is the gate that turns
// that drift into a red build.
//
// Copy this file into every plugin repository. It is the one test that cannot be
// satisfied by a plugin that lies about itself.
func TestManifestInterfacesAreServedAsDeclared(t *testing.T) {
	for _, iface := range loadManifestInterfaces(t) {
		for _, method := range iface.Methods {
			resp := handle(request{
				Action:  "call",
				Service: iface.Service,
				Method:  method.Name,
				Payload: map[string]any{},
			})
			served := !refusedAsUnknown(resp)

			switch iface.Backing {
			case "runtime":
				// This artifact is the declared owner, so it must at least recognise the
				// method. Rejecting an empty payload is a real answer; not knowing the
				// method at all is a broken promise.
				if !served {
					t.Errorf("%s/%s is declared runtime-backed, but this artifact does not serve it: %s",
						iface.Service, method.Name, resp.Error)
				}
			case "core":
				// The engine lives in lattice-server. If the artifact answers as well, the
				// manifest names two owners for one method and the host has to guess.
				if served {
					t.Errorf("%s/%s is declared core-backed, but this artifact answers it too; backing must name exactly one owner",
						iface.Service, method.Name)
				}
			case "":
				t.Errorf("%s/%s declares no backing, so who serves it is left to inference",
					iface.Service, method.Name)
			default:
				t.Errorf("%s/%s declares unknown backing %q", iface.Service, method.Name, iface.Backing)
			}
		}
	}
}

// refusedAsUnknown separates "I do not implement this" from "I implement this and your
// payload is wrong". Only the former means the artifact cannot serve the method — a
// validation error proves the method is wired up.
func refusedAsUnknown(resp response) bool {
	if resp.OK {
		return false
	}
	return strings.Contains(resp.Error, "unsupported action") ||
		strings.Contains(resp.Error, "unsupported service") ||
		strings.Contains(resp.Error, "unsupported method")
}

type manifestInterface struct {
	Service string `json:"service"`
	Backing string `json:"backing"`
	Methods []struct {
		Name string `json:"name"`
	} `json:"methods"`
}

func loadManifestInterfaces(t *testing.T) []manifestInterface {
	t.Helper()
	raw, err := os.ReadFile("../manifest.json")
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m struct {
		Interfaces []manifestInterface `json:"interfaces"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if len(m.Interfaces) == 0 {
		t.Fatal("manifest declares no interfaces to verify")
	}
	return m.Interfaces
}
