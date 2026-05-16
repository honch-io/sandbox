# Honch Sandbox CLI

Developer-only CLI for validating Honch SDK behavior against a real local Honch
stack.

The sandbox is intentionally end to end: a live SDK harness sends real HTTP/CBOR
to a local proxy, the proxy forwards to capture, worker processes the event, and
ClickHouse is queried for the final ingested rows.

V1 ships the `c-core` C/POSIX adapter only.

## Build

Build the CLI from this directory:

```sh
go build -o honch ./cmd/honch
```

Run commands from `tools/sandbox`:

```sh
./honch --help
./honch sandbox --help
```

Use `--plain` or `NO_COLOR=1` when you want unstyled output for scripts or
logs:

```sh
./honch --plain sandbox status
NO_COLOR=1 ./honch sandbox status
```

## Repository Layout

The CLI expects the local Honch repos to sit beside this SDK repo:

```text
honch-io/
  SDK/
  capture/
  platform/
  worker/
```

Defaults are defined in `config/default.yaml`:

- capture repo: `../capture`
- platform repo: `../platform`
- worker repo: `../worker`
- capture port: `8001`
- worker port: `8080`
- ClickHouse port: `8123`
- proxy port: `18080`

Override settings from the SDK repo root with `.honch-sandbox.yaml` when your
local checkout differs.

## Quickstart

Start the real stack:

```sh
./honch sandbox start
```

The command asks before running platform database migrations:

```text
Run platform database migrations with `bun run db:migrate`? [y/N]
```

Answer `yes` when you want the sandbox to apply migrations and seed the sandbox
project.

Check health:

```sh
./honch sandbox status
```

Run the C/POSIX harness in the background:

```sh
./honch sandbox run c-core --detach
```

Send a few live controls:

```sh
./honch sandbox battery --level 8
./honch sandbox track camera.motion --properties '{"zone":"porch"}'
./honch sandbox flush
```

Verify the event reached ClickHouse:

```sh
./honch sandbox events list
```

Stop everything:

```sh
./honch sandbox stop
```

## Contributor Smoke Test

Use this after SDK changes when you want a fast sanity check that queueing,
transport, capture, worker, and ClickHouse ingestion still work together.

```sh
cd tools/sandbox

go build -o honch ./cmd/honch

./honch sandbox start
./honch sandbox run c-core --detach

./honch sandbox battery --level 8
./honch sandbox track sdk.smoke --properties '{"source":"manual"}'
./honch sandbox flush

./honch sandbox events list
./honch sandbox stop
```

The tracked event should appear in `events list`. A low battery level also emits
the SDK battery event, so seeing both rows is expected.

## Network And Retry Testing

The SDK still uses real HTTP. Network modes are implemented by the local proxy.

Test offline queueing:

```sh
./honch sandbox network --offline

./honch sandbox track sdk.offline --properties '{"source":"manual"}'
./honch sandbox flush

./honch sandbox network --online
./honch sandbox flush

./honch sandbox events list
```

The first flush should fail in the harness log while offline. After returning
online and flushing again, the queued event should appear in ClickHouse.

Test server errors:

```sh
./honch sandbox network --server-error
./honch sandbox flush

./honch sandbox network --online
./honch sandbox flush
```

## Scenarios

Scenarios are YAML files that run repeatable harness and proxy controls.

Example:

```yaml
name: battery retry check
steps:
  - battery:
      level: 7
  - network:
      mode: offline
  - track:
      event: camera.motion
      properties:
        zone: porch
  - flush: {}
  - network:
      mode: online
  - flush: {}
```

Run it:

```sh
./honch sandbox scenario run ./scenario.yaml
./honch sandbox events list
```

Supported step types:

- `battery`: set the harness battery level.
- `network`: set `online`, `offline`, or `server-error`.
- `track`: emit a custom event with optional properties.
- `flush`: ask the SDK to flush queued events.
- `reset`: run SDK reset behavior.
- `wait`: pause, for example `duration: 1s`.

## Logs

Print recent logs:

```sh
./honch sandbox logs device
./honch sandbox logs stack
./honch sandbox logs proxy
```

Log files live under `.honch-sandbox/logs`.

## Command Reference

```sh
./honch sandbox start
```

Starts platform Docker Compose services, asks before running migrations, seeds
the sandbox project, starts capture/worker, starts the proxy, and records a
managed sandbox session.

```sh
./honch sandbox status
```

Shows session state, active runner, proxy mode, sibling repo cleanliness,
service health, and key ports.

```sh
./honch sandbox run c-core [--detach]
```

Builds and runs the C/POSIX harness. Use `--detach` for a background runner that
can be controlled by other sandbox commands.

```sh
./honch sandbox battery --level <0-100>
```

Sets the live harness battery level.

```sh
./honch sandbox network --online
./honch sandbox network --offline
./honch sandbox network --server-error
```

Controls proxy behavior.

```sh
./honch sandbox track <event> --properties '<json-object>'
```

Asks the harness to emit a custom event.

```sh
./honch sandbox flush
```

Asks the harness to flush queued events.

```sh
./honch sandbox reset
```

Asks the harness to run SDK reset behavior.

```sh
./honch sandbox events list
./honch sandbox events tail
```

Queries recent real ClickHouse rows for the seeded sandbox project.

```sh
./honch sandbox logs [stack|device|proxy]
```

Prints recent stack, harness, or proxy logs.

```sh
./honch sandbox scenario run <file.yaml>
```

Runs a repeatable YAML scenario against the live stack and harness.

```sh
./honch sandbox update
```

Fetches and fast-forwards clean sibling repos only. If `capture`, `platform`, or
`worker` has local changes, update stops before pulling anything.

```sh
./honch sandbox stop
```

Stops the active session, harness, proxy, capture/worker processes, and platform
Docker Compose services.

## Troubleshooting

If `events list` does not show a just-flushed event immediately, wait a few
seconds and run it again. Worker ingestion is asynchronous.

Check service health:

```sh
./honch sandbox status
```

Check logs:

```sh
./honch sandbox logs device
./honch sandbox logs stack
./honch sandbox logs proxy
```

If the stack is in a bad local state, stop and restart:

```sh
./honch sandbox stop
./honch sandbox start
```

If `update` refuses to run with `dirty worktree`, commit, stash, or manually
handle your changes in the named sibling repo first. The sandbox will not reset
or overwrite dirty worktrees.

## Release Boundary

Everything under `tools/sandbox/**` is developer tooling. Do not package it into
customer SDK release archives for `c-core`, `esp-idf`, or `micropython`.

Keep sandbox harnesses, fake devices, orchestration code, and sandbox configs in
`tools/sandbox`, not inside customer SDK package paths.

## V1 Scope

Included in V1:

- Go CLI using the local Honch stack.
- C/POSIX `c-core` harness.
- Real SDK HTTP/CBOR flow through capture, worker, and ClickHouse.
- Proxy-controlled online/offline/server-error behavior.
- Manual scenario execution.

Not included in V1:

- MicroPython or ESP-IDF adapters.
- Heavy full-stack CI.
- Customer-facing release artifacts.
