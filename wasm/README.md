# Wasm Plugin Template

Wasm plugins are reserved for third-party extension code that should run in a
sandbox. Lattice will use an explicit host API model:

- No filesystem access by default.
- No process execution.
- No environment variable access.
- No arbitrary network access.
- KV/static/notify APIs only when the manifest grants the matching capability.

The current MVP validates manifests but does not execute Wasm yet. Keep new
third-party plugin designs compatible with this capability model.

