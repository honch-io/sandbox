# Honch Sandbox CLI

Developer-only CLI for validating Honch SDK behavior against a real local Honch
stack.

The sandbox is intentionally end to end: a live SDK harness sends real HTTP/CBOR
to a local proxy, the proxy forwards to capture, worker processes the event, and
ClickHouse is queried for the final ingested rows.

The sandbox ships a `c-core` C/POSIX adapter and an `esp-idf` adapter that
builds real ESP-IDF firmware and runs it with Espressif's QEMU ESP32 emulator.

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

For ESP-IDF/QEMU runs, let the CLI check or install the tools:

```sh
./honch sandbox doctor
./honch sandbox setup --dry-run
./honch sandbox qemu doctor
./honch sandbox qemu install
```

`qemu install` asks before downloading anything. By default it creates a
managed ESP-IDF checkout at `.honch-sandbox/toolchains/esp-idf`, installs ESP-IDF
for ESP32, and installs Espressif's `qemu-xtensa` and `qemu-riscv32` tools. If
you already have ESP-IDF, export `IDF_PATH` or pass `--idf-path`.

Fresh machines still need the baseline system tools that the installer cannot
reasonably bootstrap itself: `git`, Python, and Homebrew on macOS. On macOS,
`qemu install` uses Homebrew to install Espressif's documented QEMU runtime
libraries before installing the ESP-IDF QEMU tools.

Use `sandbox doctor` when setting up a new machine. It checks host tools,
sibling repos, and emulator readiness, then prints the next missing setup steps.
Use `sandbox setup` to print or run supported installer actions. It always shows
the commands first, and asks before running unless `--yes` is passed.

The local stack depends on Docker images for Postgres, Redis, ClickHouse, and the
Pub/Sub emulator. Pull them explicitly on a new machine, or when Docker reports
image-store or missing-blob errors:

```sh
./honch sandbox images list
./honch sandbox images pull
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
./honch sandbox doctor
./honch sandbox images pull
./honch sandbox start
```

The command asks before running platform database migrations:

```text
Run platform database migrations with `bun run db:migrate`? [y/N]
```

Answer `yes` when you want the sandbox to apply migrations before seeding the
sandbox project. Answer `no` to start with the current database schema. For
non-interactive use, pass `--migrate` or `--skip-migrations`.

Running `start` again while a sandbox session is already active is a no-op.

Check health:

```sh
./honch sandbox status
```

Run the C/POSIX harness in the background:

```sh
./honch sandbox run c-core --detach
```

Or run the ESP-IDF firmware in QEMU:

```sh
./honch sandbox run esp-idf --detach
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

## Editing Harness Product Code

Each adapter harness is split into customer-like app code and sandbox plumbing.
Edit the app code when you want to test how a customer would use the SDK. Avoid
editing the plumbing unless you are changing how the CLI talks to the harness.

```text
tools/sandbox/harnesses/c-core/
  app.c              # customer-like C/POSIX SDK integration under test
  app.h
  main.c             # small entrypoint that wires env/config and control
  sandbox_control.c  # CLI JSON/FIFO control plumbing
  sandbox_control.h

tools/sandbox/harnesses/esp-idf/main/
  app.c              # customer-like ESP-IDF SDK integration under test
  app.h
  app_main.c         # firmware entrypoint
  sandbox_control.c  # UART JSON control plumbing used by QEMU
  sandbox_control.h
  sandbox_network.c  # QEMU OpenETH setup
  sandbox_network.h
```

Typical SDK contributor loop:

```sh
cd tools/sandbox

# Build the CLI only when Go CLI code changed.
go build -o honch ./cmd/honch

# Edit customer-like harness behavior.
$EDITOR harnesses/c-core/app.c
# or:
$EDITOR harnesses/esp-idf/main/app.c

# Rerun the adapter. The CLI rebuilds the harness or firmware for you.
./honch sandbox run c-core --detach
# or:
./honch sandbox run esp-idf --detach

# Drive the simulated device behavior.
./honch sandbox battery --level 8
./honch sandbox track camera.motion --properties '{"zone":"porch"}'
./honch sandbox flush
./honch sandbox events list
```

For example, if you add SDK behavior around battery changes, put the
customer-like reaction in `app.c`, then use `honch sandbox battery --level 8` to
drive the callback path. The command goes through the sandbox control file, but
the SDK behavior being tested stays in the app file.

## ESP-IDF QEMU Smoke Test

Use this when changing the ESP-IDF SDK or shared embedded behavior. This path
builds the sandbox firmware from `tools/sandbox/harnesses/esp-idf`, links the
real local `esp-idf/honch` component, boots it through Espressif QEMU, and
drives the firmware over UART using the same JSON control commands.

```sh
cd tools/sandbox

go build -o honch ./cmd/honch

./honch sandbox qemu doctor
# If doctor reports missing tools:
./honch sandbox qemu install

./honch sandbox start
./honch sandbox run esp-idf --detach

./honch sandbox battery --level 8
./honch sandbox track sdk.esp_idf_smoke --properties '{"source":"qemu"}'
./honch sandbox flush

./honch sandbox events list
./honch sandbox logs device
./honch sandbox stop
```

The ESP-IDF runner starts QEMU with `-nic user,model=open_eth`. The firmware
uses OpenETH networking and points the SDK at `http://10.0.2.2:<proxy port>`,
which is the host address visible from QEMU user networking.

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
./honch sandbox run esp-idf [--detach]
```

Builds the ESP-IDF sandbox firmware with `idf.py`, injecting the sandbox API key
and QEMU-visible proxy endpoint at build time, then runs the firmware with
`qemu-system-xtensa` using a TCP serial control channel.

```sh
./honch sandbox adapters list
./honch sandbox adapters show esp-idf
./honch sandbox adapters doctor esp-idf
./honch sandbox adapters validate
```

Inspects and validates registered adapter configs from `tools/sandbox/adapters`.
Adapter commands are read-only except for the underlying tool checks performed
by `doctor`.

```sh
./honch sandbox qemu doctor
./honch sandbox qemu install [--idf-path <path>] [--ref v6.0.1] [--yes]
```

Checks or installs the ESP-IDF/QEMU toolchain needed by `run esp-idf`. The
managed install path is used automatically by later sandbox runs, so contributors
do not need to keep `IDF_PATH` exported after installing through the CLI.

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

See `TRUST.md` for the setup, installer, repo-update, and adapter safety rules
that keep the public CLI inspectable and predictable.

See `adapters/README.md` for the current adapter schema and the expected future
MicroPython adapter direction. Do not add a new adapter name to the registry
until it has a real harness and an E2E verification path.

## Scope

Included:

- Go CLI using the local Honch stack.
- C/POSIX `c-core` harness.
- ESP-IDF `esp-idf` firmware harness running under QEMU.
- Real SDK HTTP/CBOR flow through capture, worker, and ClickHouse.
- Proxy-controlled online/offline/server-error behavior.
- Manual scenario execution.

Not included:

- MicroPython adapters.
- Heavy full-stack CI.
- Customer-facing release artifacts.
