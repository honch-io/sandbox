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

## Install And First Launch

The easiest install path is the release installer:

```sh
curl -fsSL https://honch.dev | sh
```

The installer detects your OS and CPU, downloads the latest
`honch-<os>-<arch>` release binary, asks before copying it to
`~/.local/bin/honch`, prepares a sandbox checkout under
`~/.local/share/honch/sandbox`, then runs `honch onboarding` from that checkout.

Use `--no-install` if you want to try the CLI from a temporary download without
copying it into your PATH:

```sh
curl -fsSL https://honch.dev | sh -s -- --no-install
```

Use `--sandbox-dir <dir>` if you want the sandbox checkout somewhere else.

There are also manual paths:

| Path | What it is |
| --- | --- |
| Go install | Install the latest tagged CLI with `go install honch.dev/honch/cmd/honch@latest` |
| Build from git | Compile the current checkout and use the binary in place |
| Copy into PATH | Install the built binary to `~/.local/bin/honch` |
| Release binary | Download a prebuilt GitHub release artifact |

If you just want the CLI:

```sh
go install honch.dev/honch/cmd/honch@latest
honch
```

If you already cloned this repo, the shortest path is:

```sh
go build -o honch ./cmd/honch
./honch install
honch
```

`./honch install` copies the current binary to `~/.local/bin/honch` after
confirmation. After that, the first `honch` launch opens the onboarding wizard.

The wizard is the right place to:

- clone missing `capture`, `platform`, and `worker` repos
- point Honch at existing checkouts if you already have them
- run `honch sandbox setup` to install missing host tools, images, and QEMU
- copy the binary into `~/.local/bin/honch` if you skipped the install step

If you want to rerun the wizard later, use `honch onboarding`.

If you prefer a release binary, download the tagged `honch-<os>-<arch>` asset
from GitHub Releases and run the same first-launch flow after unpacking it.

Use `./honch sandbox qemu doctor` and `./honch sandbox qemu install` when you
plan to run the ESP-IDF adapter.

## What the CLI Does

| Goal | Command |
| --- | --- |
| Run the first-launch wizard | `honch onboarding` |
| Check host prerequisites | `./honch sandbox doctor` |
| Install supported tools | `./honch sandbox setup` |
| Check ESP-IDF/QEMU setup | `./honch sandbox qemu doctor` |
| Install ESP-IDF/QEMU tools | `./honch sandbox qemu install` |
| Pull local Docker images | `./honch sandbox images pull` |
| Start the sandbox stack | `./honch sandbox start` |
| Stop the sandbox stack or runner | `./honch sandbox stop` |
| Run the C/POSIX harness | `./honch sandbox run c-core --detach` |
| Run the ESP-IDF harness | `./honch sandbox run esp-idf --detach` |
| Run ESP-IDF on real hardware | `./honch sandbox run esp-idf --device /dev/cu.usbserial-0001 --erase-flash` |
| Drive the harness | `battery`, `network`, `track`, `flush`, `reset` |
| Inspect events | `./honch sandbox events list` or `./honch sandbox events tail` |
| Inspect logs | `./honch sandbox logs device`, `./honch sandbox logs proxy` |

`sandbox start` may prompt before it runs platform database migrations. Pass
`--migrate` or `--skip-migrations` when you want to control that explicitly.

Use `--plain` or `NO_COLOR=1` when you want unstyled output for scripts or
logs.

## Repository Layout

The CLI expects the local Honch repos to sit beside this sandbox checkout:

```text
honch-io/
  sandbox/
  capture/
  platform/
  worker/
```

The first-launch wizard asks for these paths if they are not already in the
default locations. It can also clone missing sibling repos into the parent
directory of this checkout.

Defaults live in `config/default.yaml`:

- capture repo: `../platform` (runs the `honch-capture` Cargo workspace package)
- platform repo: `../platform`
- worker repo: `../platform` (runs the `honch-unified-worker` Cargo workspace package)
- capture source: blank by default; capture now lives in the platform repo
- platform source: `https://github.com/honch-io/platform.git`
- worker source: blank by default; worker now lives in the platform repo
- capture port: `8001`
- worker port: `8080`
- ClickHouse port: `8123`
- proxy port: `18080`

Override those values from the sandbox repo root with `.honch-sandbox.yaml` when
your local checkout is different.

Real ESP-IDF hardware runs need the sandbox proxy to listen on an address the
device can reach. Set `sandbox.proxy_bind` to `0.0.0.0`, restart the stack, and
pass Wi-Fi credentials with flags or environment variables:

Warning: `0.0.0.0` exposes the sandbox proxy to other devices on your LAN. Use
it only while running real hardware, then switch back to loopback.

```sh
./honch sandbox config set sandbox.proxy_bind 0.0.0.0
./honch sandbox stop
./honch sandbox start

HONCH_SANDBOX_WIFI_SSID="your-ssid" \
HONCH_SANDBOX_WIFI_PASSWORD="your-password" \
  ./honch sandbox run esp-idf --device /dev/cu.usbserial-0001 --erase-flash

./honch sandbox config set sandbox.proxy_bind 127.0.0.1
./honch sandbox stop
./honch sandbox start
```

Use `./honch sandbox flags` to inspect command-specific flags without opening
each subcommand help screen.
Use `./honch sandbox config list` to inspect the resolved values, `set` to
change a single key, `edit` to open `.honch-sandbox.yaml` in your editor, and
`init` to write a starter override file. Custom ESP-IDF checkouts are stored
in `sandbox.idf_path`.

## Working The Loop

For a normal contributor loop:

1. Start the stack with `./honch sandbox start`.
2. Run the adapter with `./honch sandbox run <adapter> --detach`.
3. Drive behavior with `battery`, `network`, `track`, `flush`, or `reset`.
4. Inspect rows with `./honch sandbox events list`.
5. Stop everything with `./honch sandbox stop`.
6. Stop just the active runner with `./honch sandbox stop c-core` or `./honch sandbox qemu stop`.

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

Harnesses are developer-only and live under `harnesses/`.

The current adapters are:

- `c-core`: native POSIX C harness linked against the canonical SDK POSIX port
- `esp-idf`: ESP32 firmware harness running in Espressif QEMU

Each harness is split into customer-like app code and sandbox plumbing. Edit the
app code when you want to exercise SDK behavior. Leave the plumbing alone unless
you are changing how the CLI talks to the harness.

```text
harnesses/c-core/
  app.c              # customer-like C/POSIX SDK integration under test
  app.h
  main.c             # entrypoint wiring env/config and control
  sandbox_control.c  # CLI JSON/FIFO control plumbing
  sandbox_control.h

harnesses/esp-idf/main/
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
builds the firmware from `harnesses/esp-idf`, links the local
`SDK/ports/esp-idf/honch` component, boots it in QEMU, and drives the firmware
over UART with the same JSON control commands.

If you pass `--idf-path`, Honch stores the resolved checkout path in
`sandbox.idf_path` so later `qemu doctor` and `run esp-idf` commands use the
same toolchain without requiring `IDF_PATH` to be exported again.

```sh
go build -o honch ./cmd/honch

./honch sandbox qemu doctor
./honch sandbox qemu install

./honch sandbox start
./honch sandbox run esp-idf --detach

./honch sandbox battery --level 8
./honch sandbox track sdk.esp_idf_smoke --properties '{"source":"qemu"}'
./honch sandbox flush
./honch sandbox qemu stop

./honch sandbox events list
./honch sandbox logs device
./honch sandbox stop
```

The firmware starts with `-nic user,model=open_eth`.

For real hardware, use `--device <serial-port>`. The hardware path builds the
same firmware harness, switches it from QEMU Ethernet to Wi-Fi, flashes the
device, then opens the ESP-IDF monitor. The device endpoint is auto-derived from
the first LAN IPv4 address and the sandbox proxy port. Override it with
`--device-endpoint http://<host-ip>:<proxy-port>` if needed.

## Troubleshooting

- `sandbox run` needs a live sandbox stack.
- `sandbox network` needs a running sandbox and a valid proxy mode.
- `sandbox events tail` streams until you cancel it.
- `sandbox update` only fast-forwards clean sibling repos.

## Related Docs

- [Adapter registry](./adapters/README.md)
- [Trust model](./TRUST.md)
