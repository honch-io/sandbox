# Honch Sandbox CLI

Developer tooling for running Honch SDK harnesses against a real local Honch
stack.

This repo is for contributors, not customer releases. It contains the sandbox
CLI, the harnesses it launches, and the adapter registry that ties them
together.

The execution path is real end to end:

```text
SDK harness -> local proxy -> capture -> worker -> ClickHouse
```

## Quick Start

```sh
cd tools/sandbox
go build -o honch ./cmd/honch

./honch sandbox doctor
./honch sandbox images pull
./honch sandbox start
./honch sandbox run c-core --detach
./honch sandbox battery --level 8
./honch sandbox track camera.motion --properties '{"zone":"porch"}'
./honch sandbox flush
./honch sandbox events list
./honch sandbox stop
```

Use `./honch sandbox qemu doctor` and `./honch sandbox qemu install` when you
plan to run the ESP-IDF adapter.

## What the CLI Does

| Goal | Command |
| --- | --- |
| Check host prerequisites | `./honch sandbox doctor` |
| Install supported tools | `./honch sandbox setup` |
| Check ESP-IDF/QEMU setup | `./honch sandbox qemu doctor` |
| Install ESP-IDF/QEMU tools | `./honch sandbox qemu install` |
| Pull local Docker images | `./honch sandbox images pull` |
| Start the sandbox stack | `./honch sandbox start` |
| Stop the sandbox stack | `./honch sandbox stop` |
| Run the C/POSIX harness | `./honch sandbox run c-core --detach` |
| Run the ESP-IDF harness | `./honch sandbox run esp-idf --detach` |
| Drive the harness | `battery`, `network`, `track`, `flush`, `reset` |
| Inspect events | `./honch sandbox events list` or `./honch sandbox events tail` |
| Inspect logs | `./honch sandbox logs device`, `./honch sandbox logs proxy` |

`sandbox start` may prompt before it runs platform database migrations. Pass
`--migrate` or `--skip-migrations` when you want to control that explicitly.

Use `--plain` or `NO_COLOR=1` when you want unstyled output for scripts or
logs.

## Repository Layout

The CLI expects the local Honch repos to sit beside this SDK repo:

```text
honch-io/
  SDK/
  capture/
  platform/
  worker/
```

Defaults live in `config/default.yaml`:

- capture repo: `../capture`
- platform repo: `../platform`
- worker repo: `../worker`
- capture port: `8001`
- worker port: `8080`
- ClickHouse port: `8123`
- proxy port: `18080`

Override those values from the SDK repo root with `.honch-sandbox.yaml` when
your local checkout is different.

## Working The Loop

For a normal contributor loop:

1. Start the stack with `./honch sandbox start`.
2. Run the adapter with `./honch sandbox run <adapter> --detach`.
3. Drive behavior with `battery`, `network`, `track`, `flush`, or `reset`.
4. Inspect rows with `./honch sandbox events list`.
5. Stop everything with `./honch sandbox stop`.

Examples:

```sh
./honch sandbox battery --level 8
./honch sandbox track sdk.smoke --properties '{"source":"manual"}'
./honch sandbox network --offline
./honch sandbox flush
./honch sandbox events list
```

The tracked event should appear in `events list`. A low battery level also
emits the SDK battery event, so seeing both rows is expected.

## Harnesses

Harnesses are developer-only and live under `tools/sandbox/harnesses`.

The current adapters are:

- `c-core`: native POSIX C harness
- `esp-idf`: ESP32 firmware harness running in Espressif QEMU

Each harness is split into customer-like app code and sandbox plumbing. Edit the
app code when you want to exercise SDK behavior. Leave the plumbing alone unless
you are changing how the CLI talks to the harness.

```text
tools/sandbox/harnesses/c-core/
  app.c              # customer-like C/POSIX SDK integration under test
  app.h
  main.c             # entrypoint wiring env/config and control
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

Typical contributor loop:

```sh
cd tools/sandbox
go build -o honch ./cmd/honch

$EDITOR harnesses/c-core/app.c
# or:
$EDITOR harnesses/esp-idf/main/app.c

./honch sandbox run c-core --detach
./honch sandbox battery --level 8
./honch sandbox track camera.motion --properties '{"zone":"porch"}'
./honch sandbox flush
./honch sandbox events list
```

## ESP-IDF QEMU

Use this path when changing the ESP-IDF SDK or shared embedded behavior. It
builds the firmware from `tools/sandbox/harnesses/esp-idf`, links the local
`esp-idf/honch` component, boots it in QEMU, and drives the firmware over UART
with the same JSON control commands.

```sh
cd tools/sandbox
go build -o honch ./cmd/honch

./honch sandbox qemu doctor
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

The firmware starts with `-nic user,model=open_eth`.

## Troubleshooting

- `sandbox run` needs a live sandbox stack.
- `sandbox network` needs a running sandbox and a valid proxy mode.
- `sandbox events tail` streams until you cancel it.
- `sandbox update` only fast-forwards clean sibling repos.

## Related Docs

- [Adapter registry](./adapters/README.md)
- [Trust model](./TRUST.md)
