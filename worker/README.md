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

