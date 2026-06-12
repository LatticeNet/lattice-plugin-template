# Worker Plugin Template

Worker plugins are intentionally small. They render responses through approved
capabilities and should not assume filesystem, process, environment, or arbitrary
network access.

Example source:

```txt
hello {{path}} {{kv:default/message}}
```

Suggested manifest:

```json
{
  "id": "example.worker",
  "name": "Example Worker",
  "type": "worker",
  "capabilities": ["worker:route", "kv:read"]
}
```

Worker plugins are deliberately narrow. The server accepts only these
capabilities for `type: "worker"`:

- `worker:route`
- `kv:read`
- `static:read`

Use a trusted `system` plugin when a feature needs host, process, network-plan,
DDNS, tunnel, or remote-task capabilities.
