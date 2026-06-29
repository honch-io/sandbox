#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="$ROOT/scripts/remote-dev"

# Run against fixed, machine-independent config so the dry-run output is
# deterministic regardless of any local .remote-dev.env on the developer's box.
export REMOTE_DEV_ENV_FILE=/dev/null
export REMOTE_HOST="tester@build-host"
export REMOTE_ROOT="/work/honch-io"

assert_contains() {
  local haystack="$1"
  local needle="$2"
  if [[ "$haystack" != *"$needle"* ]]; then
    printf 'expected output to contain %q\n' "$needle" >&2
    printf 'actual output:\n%s\n' "$haystack" >&2
    exit 1
  fi
}

help_output="$("$SCRIPT" help)"
assert_contains "$help_output" "Usage: remote-dev <command>"
assert_contains "$help_output" "sync"
assert_contains "$help_output" "remote"
assert_contains "$help_output" "remote-tty"
assert_contains "$help_output" "esp32"
assert_contains "$help_output" "pico"

env_output="$("$SCRIPT" --dry-run print-env)"
assert_contains "$env_output" "REMOTE_HOST="
assert_contains "$env_output" "REMOTE_ROOT="
assert_contains "$env_output" "ESP32_PORT="
assert_contains "$env_output" "PICO_PORT="
assert_contains "$env_output" "CLICKHOUSE_NATIVE_PORT="

dry_output="$("$SCRIPT" --dry-run sync)"
assert_contains "$dry_output" "ssh "
assert_contains "$dry_output" "rsync "
assert_contains "$dry_output" "--exclude .honch-sandbox"
assert_contains "$dry_output" "--exclude .honch-sandbox.yaml"
assert_contains "$dry_output" "--exclude /honch"
assert_contains "$dry_output" "/work/honch-io/SDK/"
assert_contains "$dry_output" "/work/honch-io/sandbox/"

setup_output="$("$SCRIPT" --dry-run setup)"
assert_contains "$setup_output" "\$HOME/.espressif/python_env"
assert_contains "$setup_output" "drizzle-kit"
if [[ "$setup_output" == *"$HOME/.espressif/python_env"* ]]; then
  printf 'setup dry-run expanded local HOME into remote command:\n%s\n' "$setup_output" >&2
  exit 1
fi

start_output="$("$SCRIPT" --dry-run start)"
assert_contains "$start_output" "19000:9000"
assert_contains "$start_output" "127.0.0.1:8085"

remote_tty_output="$("$SCRIPT" --dry-run remote-tty "echo needs tty")"
assert_contains "$remote_tty_output" "ssh -tt "
assert_contains "$remote_tty_output" "needs"
assert_contains "$remote_tty_output" "tty"

pico_output="$("$SCRIPT" --dry-run pico)"
assert_contains "$pico_output" "\$HOME/.espressif/python_env"
if [[ "$pico_output" == *"$HOME/.espressif/python_env"* ]]; then
  printf 'pico dry-run expanded local HOME into remote command:\n%s\n' "$pico_output" >&2
  exit 1
fi

esp32_output="$("$SCRIPT" --dry-run esp32 --wifi-ssid "Pilot Network" --wifi-password "Pilot Password")"
assert_contains "$esp32_output" "Pilot"
assert_contains "$esp32_output" "Network"
assert_contains "$esp32_output" "--after\\ no-reset"
assert_contains "$esp32_output" "ESP32\\ ready\\ marker\\ observed"
