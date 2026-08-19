# Plugin Security

This repository is a reference implementation for a first-party Lattice Bundle
v2 plugin. Treat it as an example of the minimum safety bar, not as a shortcut
around review.

## Ownership And Trust

- `manifest.json` is checked in unsigned, and an unsigned manifest does not
  load. `VerifyManifest` sets `requireSignature` for every
  `lattice.plugin.manifest.v2` manifest, so this bundle is rejected with
  "manifest signature requires publisher" on any server, including a local one.
  The dev-only escape hatch `AllowUnsignedHostRisk` does not cover it: that flag
  only relaxes the v1 host-risk branch. To run this template you must set a
  `publisher`, sign it with `pluginsign`, and add that publisher to the server's
  `-plugin-trust` file.
- Production bundles must replace the placeholder digest with the real
  artifact SHA-256 and add a trusted signature in release automation.
- The published trust decision should bind the manifest identity to the exact
  packaged bytes, not to an unpacked source checkout.

## Runtime Isolation

- The Go runtime is a separate stdio process with explicit JSON inputs and
  outputs.
- The UI runs in a sandboxed iframe and uses only the postMessage bridge with a
  nonce from `location.hash`.
- The reference UI does not use `fetch`, XHR, local/session storage, cookies,
  inline scripts, inline styles, or top-level navigation.
- Built assets must stay self-contained under `ui/index.html` and `ui/assets/`.

## Packaging Guarantees

- `tools/pluginpack` emits deterministic `tar.gz` archives with sorted paths.
- Tar headers use Unix-epoch timestamps and zero uid/gid metadata.
- Directories and runtime binaries use mode `0700`; other files use `0600`.
- Unsafe archive names, symlinks, and unsupported filesystem entry types are
  rejected before packaging.

## Review Expectations

- Keep runtime entrypoints limited to the manifest-declared Linux binaries.
- Keep UI changes auditable and avoid adding any dependency on `lattice-dashboard`
  internals.
- Treat `network:plan` as a reviewed scope. Expanding to mutating or host-risk
  scopes should come with new interface metadata, tests, and explicit signer
  approval.
