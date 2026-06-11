# Lattice Plugin Template

This directory is the starting point for new Lattice extensions.

Plugin categories:

- `system-go/` - trusted system plugin process template for host operations.
- `worker/` - capability-limited dashboard/server Worker template.
- `wasm/` - future Wasm plugin template notes.

## Manifest

Every plugin must declare an explicit capability list:

```json
{
  "id": "example.nft-guard",
  "name": "Example nft Guard",
  "type": "system",
  "capabilities": ["network:plan"]
}
```

Unknown capabilities are rejected by the server. Dangerous capabilities such as
`network:apply` should never be granted to third-party plugins by default.

## System Plugin Contract

The bootstrap template uses newline-delimited JSON over stdio:

Input:

```json
{"action":"plan","payload":{"public_tcp":[80,443]}}
```

Output:

```json
{"ok":true,"plan":"...","message":"plan generated"}
```

Future Lattice releases can replace stdio with gRPC/hashicorp-go-plugin while
keeping the same capability and approval semantics.

