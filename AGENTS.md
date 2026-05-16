# Sandbox Tooling Instructions

These instructions apply to `tools/sandbox/**`.

## Scope

- This directory is developer tooling for Honch SDK contributors.
- Do not put sandbox harnesses, fake devices, stack orchestration, or sandbox
  config inside customer SDK package paths such as `c-core/`, `esp-idf/`, or
  `micropython/`.
- Sandbox code must not be included in per-SDK customer release artifacts.

## Architecture

- Keep the Go CLI cleanly separated into command wiring and internal packages.
- Cobra commands should parse flags and delegate real work to `internal/...`.
- Viper-backed config must remain typed. Avoid stringly typed config passing.
- Keep SDK adapters data-driven where possible. New adapters should fit beside
  `adapters/c-core.yaml` without hardcoding SDK-specific behavior into command
  handlers.
- Use detailed comments only for complex orchestration, process lifecycle, or
  cross-repo behavior that is not obvious from the code.

## E2E Rules

- The sandbox is for real end-to-end SDK validation: real SDK process, real
  HTTP/CBOR, real capture, real worker, and real ClickHouse assertions.
- Do not replace the stack with mocks for command behavior that claims to be
  E2E.
- Local stack repos default to `../capture`, `../platform`, and `../worker`.
- `sandbox update` must only fetch and fast-forward clean sibling repos. Never
  reset, clean, rebase, or overwrite dirty worktrees.

## Harness Rules

- Harnesses are developer-only and belong under `tools/sandbox/harnesses`.
- C/POSIX is the only V1 adapter. Do not add MicroPython or ESP-IDF commands
  until their adapter contracts are explicitly designed.
- Harness controls use newline-delimited JSON so future adapters can share the
  same command surface.
