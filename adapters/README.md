# Sandbox Adapters

Adapters describe SDK harnesses that `honch sandbox run <adapter>` can launch.
The registry is intentionally config-first so new SDK targets do not require
hardcoding command names throughout the CLI.

Supported adapters today:

- `c-core`: native POSIX C harness. No emulator required.
- `esp-idf`: ESP32 firmware harness running in Espressif QEMU.

## Schema

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
  path: session-runner-control-fifo
events:
  source: real-sdk-http-cbor
  sink: real-clickhouse
```

`name` is the public CLI identifier. `kind` selects the runner implementation.
The only runner kinds supported now are `posix` and `qemu-esp32`.

`harness` must point at a developer-only harness under `tools/sandbox`, not a
customer SDK package path. The harness should link or import the local SDK under
test, emit real SDK events, and accept live controls through the declared
control transport.

## Future MicroPython Direction

MicroPython is expected to become its own adapter, but it should not be added as
a placeholder command before there is a real harness and verification path.

Expected shape:

```yaml
name: micropython
kind: micropython-qemu
harness: harnesses/micropython
build:
  tool: python
run:
  tool: qemu-system-...
controls:
  transport: newline-json-uart
events:
  source: real-sdk-http-cbor
  sink: real-clickhouse
```

Before enabling it in the registry, define the actual emulator target, build
artifact, control transport, and SDK import path. It must preserve the same E2E
contract as `c-core` and `esp-idf`: real SDK process, real HTTP/CBOR, real
capture, real worker, and ClickHouse assertions.
