# Sandbox Adapters

Adapters describe SDK harnesses that `honch sandbox run <adapter>` can launch.
The registry is config-first so new SDK targets do not require hardcoding
command names throughout the CLI.

## Supported Adapters

| Name | Kind | Harness | Emulator |
| --- | --- | --- | --- |
| `c-core` | `posix` | `harnesses/c-core` | none |
| `esp-idf` | `qemu-esp32` | `harnesses/esp-idf` | Espressif QEMU |

## Registry Contract

Each adapter entry describes:

- `name`: public CLI identifier
- `kind`: runner implementation to use
- `harness`: developer-only harness path under `harnesses`
- `build`: how to produce the harness artifact
- `run`: how to launch the artifact
- `emulator`: only for adapters that need one
- `controls`: how the CLI sends live commands
- `events`: where SDK events are expected to land

Example:

```yaml
name: esp-idf
kind: qemu-esp32
harness: harnesses/esp-idf
build:
  tool: idf.py
  target: esp32
  output: qemu-flash-image
run:
  tool: qemu-system-xtensa
  serial: tcp
emulator:
  tool: qemu-system-xtensa
  machine: esp32
  network: open_eth
controls:
  transport: newline-json-uart
events:
  source: real-sdk-http-cbor
  sink: real-clickhouse
```

## Adding An Adapter

Before an adapter appears in the registry, it should have:

1. A real harness under `harnesses/**`
2. A concrete build path
3. A concrete run path
4. A documented control transport
5. A real events path that ends in ClickHouse
6. A working `sandbox run <adapter>` flow

Do not add placeholder adapter names or speculative entries. An adapter should
only become visible when there is a real E2E verification route behind it.

## Notes

- Harnesses must stay in this repo under `harnesses/`, not customer SDK package paths.
- `c-core` is the fast POSIX smoke-test adapter.
- `esp-idf` runs as firmware under QEMU.
- Keep future adapters behind the same `sandbox run <adapter>` contract and
  document their emulator assumptions before exposing commands.
