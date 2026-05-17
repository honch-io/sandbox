# Sandbox Trust Model

The sandbox is developer tooling for Honch SDK contributors. It should be
predictable to run, explicit about machine changes, and isolated from customer
release artifacts.

## Setup And Install Safety

- `honch sandbox setup --dry-run` prints supported setup actions.
- `honch sandbox setup` asks before running setup actions.
- `honch sandbox qemu install --dry-run` prints ESP-IDF/QEMU install commands.
- `honch sandbox qemu install` asks before downloading or installing tools.

`--yes` is allowed for scripted setup, but it must never be the default.

The CLI may install managed ESP-IDF/QEMU tooling under
`.honch-sandbox/toolchains`. Baseline system tools such as Homebrew, Python,
Docker, Bun, Rust/Cargo, and CMake are reported by `honch sandbox doctor`.

## Repository Safety

Sibling repo updates are manual:

```sh
honch sandbox update
```

`update` must only fetch and fast-forward clean sibling repos. It must not
reset, clean, force checkout, or overwrite dirty worktrees.

The sandbox expects sibling repos beside the SDK checkout:

```text
honch-io/
  SDK/
  capture/
  platform/
  worker/
```

## Release Boundary

Everything under `tools/sandbox/**` is developer tooling. It must not be
shipped inside customer SDK packages or embedded SDK release artifacts.

Sandbox harnesses may live under `tools/sandbox/harnesses/**` and may link
against local SDK code. They should not be moved into customer package paths
just to support the CLI.

## Adapter Safety

Adapter configs live under `tools/sandbox/adapters/**`. Adding a config file
should not by itself create hidden installation side effects. Installer logic
must remain explicit in command code and must have tests covering confirmation
or dry-run behavior.

Do not add placeholder adapter names to the registry. A new adapter should
have a real harness, live control path, and E2E verification route before it
becomes visible as `honch sandbox run <adapter>`.
