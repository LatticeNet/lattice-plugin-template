# Lattice Bundle v2 Reference Plugin

`lattice-plugin-template` is the canonical self-contained Bundle v2 reference
plugin for Lattice `0.2.1-alpha.1`.

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
(cd system-go && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "$tmpdir/bundle/bin/linux-amd64/plugin" .)
(cd system-go && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -o "$tmpdir/bundle/bin/linux-arm64/plugin" .)
cp -R ui/dist/. "$tmpdir/bundle/ui"
(cd tools/pluginpack && go run ./cmd/pluginpack -source "$tmpdir/bundle" -output "$tmpdir/reference-plugin.tar.gz")
```

`pluginpack` normalizes archive paths, rejects unsafe names and symlinks, stamps
tar entries at the Unix epoch, zeros uid/gid, and uses mode `0700` for
directories and runtime binaries (`bin/**/plugin`) with `0600` for other files.

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
