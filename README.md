# Lattice Bundle v2 Reference Plugin

`lattice-plugin-template` is the canonical self-contained Bundle v2 reference
plugin for Lattice `0.2.1-alpha.4`.

It demonstrates four boundaries that production plugins must keep explicit:

- ownership: the manifest, runtime, and UI are packaged together and hashed as a
  single reviewable artifact.
- isolation: the UI is a sandboxed iframe that talks only through the bridge
  nonce in `location.hash`, never through `fetch`, storage, cookies, or top-level
  navigation.
- determinism: `tools/pluginpack` emits byte-identical `tar.gz` bundles even
  when source mtimes change.
- signing discipline: the release manifest is bound to the checked-in alpha
  artifact digest and signed by the LatticeNet publisher key. Local development
  must clear the digest and signature before repackaging different bytes.

## The host-risk operation flow (spec §9.3)

A plugin may compile intent into a plan; it may never apply one. This reference
implements the full flow, and it is the shape production plugins copy:

1. **plan** — the `plan`-effect interface method (`example.lattice-plugin/reference`
   / `plan`) returns a `PluginOperationPlan`: a summary, the target nodes, a redacted
   preview, ordered steps, a rollback statement, and opaque `data`. It applies
   nothing. The server bounds it, authorizes every target, and stores it as a
   **pending approval** whose typed columns record the plugin version, artifact
   digest, service, method, request hash, and targets.

2. **approve** — an operator reads the preview and approves the exact plan hash. The
   server re-checks every bound column against live state at approve/execute time; a
   plugin that was upgraded, re-signed, disabled, or whose targets are no longer
   authorized is refused.

3. **execute** — the approval executor, and nothing else, invokes the plugin's
   `execute` action with a one-time operation grant **bound on the host side**. The
   plugin never receives the grant. It reads the approved targets and calls the
   `task.enqueue` host call once per node.

4. **enqueue** — `task:run` is *eligibility*, not authorization. The grant says which
   nodes, under which approval, and how many times. The host refuses any task aimed
   at an unapproved node, past the budget, or belonging to another plugin — and
   applies the operator's own task validation, so a plugin can reach no wider an
   interpreter set or script than an operator could.

Rules a production plugin must keep:

- **Never apply directly.** The only way to change a host is `plan` → operator
  approval → `execute` → `task.enqueue`. There is no in-plugin apply.
- **Redact the preview.** Secrets never appear in a plan; put reversible material in
  the encrypted secret store (§9.4), not in the plan or a log.
- **Quote everything you interpolate into a script.** The reference single-quotes the
  approval id and node id so neither can break out of the `sh` command.
- **Declare `task:run`** to enqueue, and — for a runtime-backed operation service —
  declare the service `backing: "runtime"`.

## Bundle Layout

The packaged artifact must contain exactly the host-facing runtime and UI assets:

```text
bin/linux-amd64/plugin
bin/linux-arm64/plugin
ui/index.html
ui/assets/*
```

`manifest.json` points at those Linux entrypoints, the sandbox UI entry, and the
Bridge v1 contract (`bridge: 1`).

## Runtime Surface

The system runtime in `system-go/` is a newline-delimited JSON process using the
`stdio-json-v1` contract. The example methods are intentionally small:

- `describe` returns stable plugin metadata for host discovery.
- `health` reports runtime readiness.
- `plan` renders a deterministic dry-run plan from sorted input keys.

The manifest also advertises typed interface metadata for `example.describe` and
`example.plan`, with `network:plan` called out as the required scope for the
planner action.

## UI Sandbox Rules

The Vue UI in `ui/` is bundled by Vite and ships as static files inside the
artifact. The reference implementation intentionally avoids:

- imports from `lattice-dashboard`
- `fetch`, XHR, cookies, `localStorage`, and `sessionStorage`
- inline scripts or inline styles in built HTML
- external URLs, top navigation, or breakout attempts

The bridge client:

- reads `lattice_nonce` from the iframe URL fragment
- posts only to `window.parent`
- sends `ready`, `call`, `cancel`, and `resize`
- accepts host `init`, `result`, `error`, `theme`, and `dispose`

## Local Build And Verification

Install UI dependencies and run the required checks:

```bash
cd ui
npm ci
npm test
npm run typecheck
npm run build
npm run verify:build
```

Run Go tests within each Go module:

```bash
(cd system-go && go test -race ./...)
(cd tools/pluginpack && go test -race ./...)
```

Build a bundle workspace and package it deterministically:

```bash
tmpdir="$(mktemp -d)"
mkdir -p "$tmpdir/bundle/bin/linux-amd64" "$tmpdir/bundle/bin/linux-arm64" "$tmpdir/bundle/ui"
(cd system-go && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -buildvcs=false -o "$tmpdir/bundle/bin/linux-amd64/plugin" .)
(cd system-go && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -buildvcs=false -o "$tmpdir/bundle/bin/linux-arm64/plugin" .)
cp -R ui/dist/. "$tmpdir/bundle/ui"
(cd tools/pluginpack && go run ./cmd/pluginpack -source "$tmpdir/bundle" -output "$tmpdir/reference-plugin.tar.gz")
```

`pluginpack` normalizes archive paths, rejects unsafe names and symlinks, stamps
tar entries at the Unix epoch, zeros uid/gid, and uses mode `0700` for
directories and runtime binaries (`bin/**/plugin`) with `0600` for other files.
Release builds pin Node.js 22 and Go 1.26.4. Both toolchains are part of the
signed byte contract: changing either can alter the bundled UI or runtime even
when the source tree is unchanged.

## Signing

The checked-in manifest represents the published alpha artifact and therefore
contains both `bundle.digest_sha256` and `signature_ed25519`. The intended
release flow is:

1. build runtime binaries and static UI assets
2. package the bundle deterministically
3. compute the real `bundle.digest_sha256`
4. sign the manifest payload with a trusted publisher key in release automation

Do not sign unpacked source trees or ad hoc developer bundles. The trust decision
must be anchored to the exact packaged bytes.

When using this repository as a starting point for a new plugin, change the
plugin identity and clear both release-bound fields before the first local
package. Never reuse the reference plugin signature for modified bytes.
