# Sandbox Agent Instructions

These instructions apply to the entire `sandbox` repository.

## What This Repo Is

`sandbox` is Honch’s developer-only CLI and harness repository. It exists so
SDK contributors can run real end-to-end validation against a real local Honch
stack:

```text
SDK harness -> local proxy -> capture -> worker -> ClickHouse
```

This repository is not customer SDK code. It is the toolchain that developers
use while changing the SDK, the harnesses, or the local stack that supports
them.

The goal of this repo is simple:

1. Keep the sandbox workflow reliable and obvious.
2. Keep the E2E path real, not mocked.
3. Keep the developer loop fast enough that people will actually use it.
4. Keep sandbox-specific code out of customer SDK release artifacts.

## How To Think About The Repository

Treat this repo as a contract between three things:

- the CLI surface contributors type into
- the harnesses that simulate SDK behavior
- the local stack that records and verifies the results

If a change makes that contract less clear, more hidden, or harder to explain,
it is probably the wrong change.

This repository was split out of the SDK repo so it can grow on its own. That
means new agents should not assume the SDK repo and the sandbox repo share the
same history, release cadence, or ownership rules.

## Core Boundaries

- Keep sandbox tooling under this repository.
- Keep harnesses under `harnesses/`.
- Keep adapter definitions under `adapters/`.
- Keep user-facing docs in `README.md` and safety rules in `TRUST.md`.
- Do not move sandbox code into customer SDK package paths.
- Do not add placeholder adapters or fake E2E paths.
- Do not replace real stack interactions with mocks for commands that claim to
  validate end-to-end behavior.

The local stack still depends on sibling checkouts of:

- `capture`
- `platform`
- `worker`

Those are dependencies of the sandbox, not children of it.

## Working Rules

- Start by reading `README.md`, `TRUST.md`, and `adapters/README.md` when you
  need the operational or architectural picture.
- Run `git status --short` before changing files.
- Preserve user-owned changes. Do not overwrite unrelated work.
- Keep edits small and focused.
- Use `apply_patch` for manual edits.
- Default to ASCII unless a file already uses Unicode.
- Update docs when behavior, setup, or workflow changes.
- Verify with relevant Go tests before claiming completion.

## Safety Rules

- `honch sandbox setup` and `honch sandbox qemu install` should remain explicit
  about side effects and user confirmation.
- `honch sandbox update` must only fast-forward clean sibling repos.
- Never add hidden installation behavior to adapter config files.
- Never make the sandbox repo responsible for shipping customer SDK artifacts.

## When You Need More Context

If you are editing the CLI, harnesses, or adapter flow, inspect the nearest
command and package first. Prefer local patterns over inventing new ones.

If you are changing behavior that affects setup, stack orchestration, or
adapter launch flow, verify the change from the contributor perspective, not
just by reading the code.

If you are unsure whether a change belongs here or in a sibling repo, stop and
trace the ownership boundary before editing.

