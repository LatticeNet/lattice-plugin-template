# Lattice Plugin Template

This directory is the starting point for new Lattice extensions.

Plugin categories:

- `system-go/` - trusted first-party system plugin process template for host operations.
- `worker/` - capability-limited dashboard/server Worker template.
- `wasm/` - future sandboxed third-party Wasm plugin template notes.

## Manifest

Every plugin must declare an explicit capability list:

```json
{
  "id": "example.nft-guard",
  "name": "Example nft Guard",
  "type": "system",
  "version": "0.1.0",
  "publisher": "latticenet",
  "entrypoint": "system-go/lattice-plugin-example",
  "digest_sha256": "<artifact sha256 hex>",
  "signature_ed25519": "<base64 ed25519 signature>",
  "capabilities": ["network:plan"]
}
```

Unknown capabilities are rejected by the server. Dangerous capabilities such as
`network:apply` should never be granted to third-party plugins by default.

Manifest identity is intentionally strict:

- `id` is a stable lowercase identifier using only `a-z`, `0-9`, `.`, and `-`;
  it must not contain whitespace, slashes, path traversal, or uppercase letters.
- `name` must be printable and at most 80 characters.
- `capabilities` must be non-empty and must not contain duplicates.
- `version` and `entrypoint` are part of the public plugin contract even when a
  current template does not need generated code yet.
- `publisher` identifies the signer/trust root. Production loaders should
  accept high-risk system plugins only from trusted publishers.
- `digest_sha256` is the lowercase hex SHA-256 of the plugin artifact/package.
- `signature_ed25519` signs the canonical Lattice plugin payload for
  `id`, `name`, `type`, `version`, `entrypoint`, `publisher`, `digest_sha256`, and
  capabilities. It prevents replacing the artifact or reusing a signature for a
  different plugin id.

## Capability Guide

Low-risk read capabilities:

- `node:read`
- `monitor:read`
- `audit:read`
- `kv:read`
- `static:read`
- `task:read`

Operator-write capabilities:

- `kv:write`
- `worker:route`
- `notify:send`

High-risk host capabilities:

- `node:admin`
- `monitor:admin`
- `ddns:admin`
- `tunnel:admin`
- `static:write`
- `task:run`
- `network:plan`
- `network:apply`

Type restrictions:

- `system` plugins may request high-risk capabilities, but should be reserved
  for first-party or operator-audited plugins.
- `worker` plugins may request only `worker:route`, `kv:read`, and
  `static:read`.
- `wasm` plugins may request read and operator-write capabilities, but may not
  request high-risk host capabilities.

Third-party plugins should start with read-only capabilities. First-party system
plugins may request high-risk capabilities only when their actions produce an
auditable dry-run plan and go through the Lattice approval flow.

`task:read` is intentionally separate from `task:run`: reading task metadata and
results must not imply the ability to queue remote execution.

## Signing Model

Development manifests may omit `digest_sha256` and `signature_ed25519`, but a
production Lattice loader should verify them for any host-risk capability such as
`network:plan`, `network:apply`, `task:run`, `ddns:admin`, or `tunnel:admin`.

The server-side verifier expects:

- `publisher` to match a trusted Ed25519 public key configured by the operator.
- `digest_sha256` to match the exact plugin artifact bytes.
- `signature_ed25519` to verify against the canonical Lattice signing payload.

Trust policy JSON:

```json
{
  "allow_unsigned_host_risk": false,
  "trusted_publishers": {
    "latticenet": "base64-raw-ed25519-public-key"
  }
}
```

> Fail-closed by default: omitting `allow_unsigned_host_risk` (or setting it
> `false`) requires a trusted-publisher Ed25519 signature for **every** host-risk
> plugin. Set it `true` only for local development on a host you fully control.

Do not sign unpacked source directories casually. Build a deterministic artifact
first, hash that artifact, then sign the manifest payload for that digest.

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

The template also implements `describe` and `health`. Keep the `describe`
response aligned with `manifest.json` so runners, CI checks, and future
marketplace tooling all see the same id, name, version, and capability surface.

Future Lattice releases can replace stdio with gRPC/hashicorp-go-plugin while
keeping the same capability and approval semantics.

Plan output should be deterministic: sort keys and avoid timestamps, random IDs,
or environment-dependent text unless they are part of the reviewed mutation.
Stable dry-run text keeps approval diffs auditable across retries.
