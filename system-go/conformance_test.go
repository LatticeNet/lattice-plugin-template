package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// A manifest declares which methods a plugin exposes and who serves each one.
// Hold the artifact to that claim so interface ownership cannot drift silently.
func TestManifestInterfacesAreServedAsDeclared(t *testing.T) {
	for _, iface := range loadManifestInterfaces(t) {
		for _, method := range iface.Methods {
			resp := handle(request{
				Action:  "call",
				Payload: callPayloadRaw(t, iface.Service, method.Name),
			})
			served := !refusedAsUnknown(resp)

			switch iface.Backing {
			case "runtime":
				if !served {
					t.Errorf("%s/%s is runtime-backed, but the artifact does not serve it: %s",
						iface.Service, method.Name, resp.Error)
				}
			case "core":
				if served {
					t.Errorf("%s/%s is core-backed, but the artifact answers it too",
						iface.Service, method.Name)
				}
			case "":
				t.Errorf("%s/%s declares no backing", iface.Service, method.Name)
			default:
				t.Errorf("%s/%s declares unknown backing %q", iface.Service, method.Name, iface.Backing)
			}
		}
	}
}

func refusedAsUnknown(resp response) bool {
	if resp.OK {
		return false
	}
	return strings.Contains(resp.Error, "unsupported action") ||
		strings.Contains(resp.Error, "unsupported service") ||
		strings.Contains(resp.Error, "unsupported method")
}

func callPayloadRaw(t *testing.T, service, method string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"service": service,
		"method":  method,
		"payload": map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
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
	var manifest struct {
		Interfaces []manifestInterface `json:"interfaces"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if len(manifest.Interfaces) == 0 {
		t.Fatal("manifest declares no interfaces to verify")
	}
	return manifest.Interfaces
}
