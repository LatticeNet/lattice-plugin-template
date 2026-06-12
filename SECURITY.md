# Plugin Security

Lattice plugins must be capability-based.

Rules:

- No filesystem access unless a future host API explicitly grants it.
- No process execution in third-party plugins.
- No environment variable access by default.
- No arbitrary network access by default.
- Manifest ids must be stable lowercase identifiers, never paths or user-facing
  labels. Capability lists must be explicit, non-empty, and duplicate-free so
  reviews and audit events cannot disagree about what was granted.
- Production manifests for high-risk/system plugins must include a trusted
  `publisher`, artifact `digest_sha256`, and `signature_ed25519`. Unsigned
  development manifests are acceptable only before installation/loading.
- `network:apply`, `task:run`, `node:admin`, `ddns:admin`, `tunnel:admin`,
  `monitor:admin`, `network:plan`, and `static:write` must be treated as high-risk.
- `task:read` is read-only and must never grant task creation or remote
  execution.
- `worker` plugins may only declare `worker:route`, `kv:read`, and
  `static:read`.
- `wasm` plugins may not declare high-risk host capabilities.
- High-risk capabilities require a trusted `system` plugin.
- Plugins that affect a node must be constrained by the caller's node allowlist.
- Webhook-style plugins must use the server's guarded outbound HTTP client; they
  must not dial loopback, private, link-local, metadata, or special-use ranges.
- All privileged operations must be auditable by `lattice-server`.

System plugins are trusted built-ins. Third-party plugins should target the
future Wasm host or the restricted Worker interface.
