# Plugin Security

Lattice plugins must be capability-based.

Rules:

- No filesystem access unless a future host API explicitly grants it.
- No process execution in third-party plugins.
- No environment variable access by default.
- No arbitrary network access by default.
- `network:apply`, `task:run`, and `static:write` must be treated as high-risk.
- All privileged operations must be auditable by `lattice-server`.

System plugins are trusted built-ins. Third-party plugins should target the
future Wasm host or the restricted Worker interface.

